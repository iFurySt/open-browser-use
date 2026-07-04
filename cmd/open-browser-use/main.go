package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ifuryst/open-browser-use/internal/host"
	"github.com/ifuryst/open-browser-use/internal/wire"
	"github.com/spf13/cobra"
)

const version = "0.1.41"
const defaultChromeExtensionID = "bgjoihaepiejlfjinojjfgokghnodnhd"
const defaultCLISessionID = "obu-cli"
const defaultMCPSessionID = "obu-mcp"
const chromeWebStoreUpdateURL = "https://clients2.google.com/service/update2/crx"
const chromeWebStoreExtensionURL = "https://chromewebstore.google.com/detail/open-browser-use/" + defaultChromeExtensionID + "?utm_source=open-computer-use"
const betaExtensionPublicKey = "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAnBLT95WWVnHYH0pOBRH/eP+BWtlKVmLE/RHkERUTI2+PGDSQrbWVabmTw4CZ3yhjko04dijSX2Az8cnp65xh23Dh5mP5TCtiP9LexRFJokd8EsyeFdtKamMYr0hF1ZUc1/8ZpLnetAU65ZMB9VzHQBqpJWeUwuIvecgfRtGklDgJMjnvcq5J6pttZrzWrI/2B0BNufwsTQfEt7qLtDFPHXmUdtZfQbc2EfYFvkXLDAXicYviiocedrsAGIKUxpyQegobhUFL+tNLOuXKBpZlLFQn3xgm5CyGZwN6bueiV/S7reigVTKAMQ8BX0eacT22e8r0UzjsjkugeHOIonIvtQIDAQAB"

// LevelDB log/ldb files for chrome.storage.local are typically under 1 MB; cap
// the scan at 32 MB so a pathological file can't OOM the host.
const maxLevelDBScanBytes = 32 << 20

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if isNativeMessagingLaunch(args) {
		return runHost(host.DefaultSocketDir, "")
	}
	cmd := newRootCommand()
	cmd.SetArgs(args)
	return cmd.Execute()
}

func isNativeMessagingLaunch(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "chrome-extension://") {
			return true
		}
		if runtime.GOOS == "windows" && strings.HasPrefix(arg, "--parent-window=") {
			return true
		}
	}
	return false
}

func newRootCommand() *cobra.Command {
	var showVersion bool
	root := &cobra.Command{
		Use:           "open-browser-use",
		Short:         "Open Browser Use native host and CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if showVersion {
				fmt.Fprintln(cmd.OutOrStdout(), version)
				return nil
			}
			return renderStartupStatus(cmd.OutOrStdout())
		},
	}
	root.Flags().BoolVarP(&showVersion, "version", "v", false, "print version")
	root.AddCommand(
		newHostCommand(),
		newSetupCommand(),
		newManifestCommand(),
		newInstallManifestCommand(),
		newCallCommand(),
		newRunCommand(),
		newMCPCommand(),
		newOpenTabCommand(),
		newNavigateCommand(),
		newSimpleRPCCommand("ping", "ping", "Ping the browser backend"),
		newSimpleRPCCommand("info", "getInfo", "Print browser backend info"),
		newSimpleRPCCommand("tabs", "getTabs", "List session tabs"),
		newSimpleRPCCommand("user-tabs", "getUserTabs", "List user Chrome tabs"),
		newHistoryCommand(),
		newClaimTabCommand(),
		newFinalizeTabsCommand(),
		newNameSessionCommand(),
		newCdpCommand(),
		newMoveMouseCommand(),
		newWaitFileChooserCommand(),
		newSetFileChooserFilesCommand(),
		newSimpleRPCCommand("turn-ended", "turnEnded", "End the current browser turn"),
		newProfilesCommand(),
		newVersionCommand(),
	)
	return root
}

func newHostCommand() *cobra.Command {
	var socketDir string
	var socketPath string
	cmd := &cobra.Command{
		Use:   "host",
		Short: "Run the native messaging host relay",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runHost(socketDir, socketPath)
		},
	}
	cmd.Flags().StringVar(&socketDir, "socket-dir", host.DefaultSocketDir, "directory for SDK Unix sockets")
	cmd.Flags().StringVar(&socketPath, "socket-path", "", "explicit SDK Unix socket path")
	return cmd
}

func runHost(socketDir string, socketPath string) error {
	relay := host.NewRelay(host.Config{
		SocketDir:  socketDir,
		SocketPath: socketPath,
	}, os.Stdin, os.Stdout)
	return relay.Serve(context.Background())
}

func newSetupCommand() *cobra.Command {
	extensionID := defaultChromeExtensionID
	var binaryPath string
	var externalExtensionOutput string
	var browser string
	var noOpen bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Register Chrome integration for Open Browser Use",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := setupChrome(extensionID, binaryPath, externalExtensionOutput, browser)
			if err != nil {
				return err
			}
			status := detectBrowserExtensionForBrowser(host.DefaultSocketDir, 700*time.Millisecond, browser)
			shouldOpenStore := shouldOpenStoreSetup(status, noOpen)
			if shouldOpenStore {
				if err := openChromeWebStorePage(); err != nil {
					result.StoreOpenError = err.Error()
				} else {
					result.OpenedStore = true
				}
			}
			return renderStoreSetupResult(cmd.OutOrStdout(), result, status)
		},
	}
	cmd.Flags().StringVar(&extensionID, "extension-id", defaultChromeExtensionID, "Chrome extension id for allowed_origins")
	cmd.Flags().StringVar(&binaryPath, "path", "", "native host binary target for the stable host link")
	cmd.Flags().StringVar(&externalExtensionOutput, "external-extension-output", "", "Chrome external extension JSON output path")
	cmd.Flags().StringVar(&browser, "browser", "", "browser to register with Chrome Web Store setup (chrome or chrome-beta)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "register Chrome integration without opening the Chrome Web Store page")
	cmd.AddCommand(newSetupBetaCommand())
	return cmd
}

func newSetupBetaCommand() *cobra.Command {
	extensionID := defaultChromeExtensionID
	var binaryPath string
	var zipPath string
	var browser string
	var noOpen bool
	cmd := &cobra.Command{
		Use:   "beta",
		Short: "Register the native host and prepare the beta extension package",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedZIPPath := zipPath
			if resolvedZIPPath == "" {
				var err error
				resolvedZIPPath, err = downloadLatestReleaseZIP()
				if err != nil {
					return err
				}
			} else {
				var err error
				resolvedZIPPath, err = resolveExistingExtensionZIP(resolvedZIPPath)
				if err != nil {
					return err
				}
			}
			unpackedPath, unpackedExtensionID, err := installUnpackedExtension(resolvedZIPPath)
			if err != nil {
				return err
			}
			installZIPPath, err := writeBetaInstallZIP(unpackedPath, resolvedZIPPath)
			if err != nil {
				return err
			}
			effectiveExtensionID := extensionID
			if !cmd.Flags().Changed("extension-id") {
				effectiveExtensionID = unpackedExtensionID
			}
			if effectiveExtensionID != unpackedExtensionID {
				return fmt.Errorf("--extension-id %s does not match keyed beta ZIP extension id %s", effectiveExtensionID, unpackedExtensionID)
			}
			manifestPath, err := installNativeManifestForBrowser(effectiveExtensionID, binaryPath, "", browser)
			if err != nil {
				return err
			}
			status := detectBrowserExtensionForBrowser(host.DefaultSocketDir, 700*time.Millisecond, browser)
			status.InstallCommand = "open-browser-use setup beta"
			if strings.TrimSpace(browser) != "" {
				status.InstallCommand += " --browser " + browser
			}
			status.UpgradeCommand = status.InstallCommand
			shouldOpen := shouldOpenManualSetup(status, noOpen)
			if shouldOpen {
				if err := openChromeExtensionsPage(); err != nil {
					return err
				}
				if err := revealFile(installZIPPath); err != nil {
					return err
				}
			}
			skillUpdate := maybeUpdateInstalledSkill()
			return renderManualSetupResult(cmd.OutOrStdout(), manualSetupResult{
				NativeManifestPath: manifestPath,
				ExtensionID:        effectiveExtensionID,
				ZIPPath:            installZIPPath,
				UnpackedPath:       unpackedPath,
				OpenedChrome:       shouldOpen,
				OpenedFileManager:  shouldOpen,
				SkillUpdate:        skillUpdate,
			}, status)
		},
	}
	cmd.Flags().StringVar(&extensionID, "extension-id", defaultChromeExtensionID, "Chrome extension id for allowed_origins")
	cmd.Flags().StringVar(&binaryPath, "path", "", "native host binary target for the stable host link")
	cmd.Flags().StringVar(&zipPath, "zip", "", "existing extension zip path; defaults to the latest GitHub Release zip")
	cmd.Flags().StringVar(&browser, "browser", "", "browser to register (chrome, chrome-beta, bitbrowser, or BitBrowser instance id)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "download and unpack the extension without opening Chrome")
	return cmd
}

func newManifestCommand() *cobra.Command {
	extensionID := defaultChromeExtensionID
	var hostPath string
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Print a Chrome native messaging host manifest",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			manifest, err := nativeManifest(extensionID, hostPath)
			if err != nil {
				return err
			}
			return writeJSON(manifest)
		},
	}
	cmd.Flags().StringVar(&extensionID, "extension-id", defaultChromeExtensionID, "Chrome extension id for allowed_origins")
	cmd.Flags().StringVar(&hostPath, "path", "", "native host path written to manifest; defaults to the stable host link")
	return cmd
}

func newInstallManifestCommand() *cobra.Command {
	extensionID := defaultChromeExtensionID
	var binaryPath string
	var outputPath string
	var browser string
	cmd := &cobra.Command{
		Use:   "install-manifest",
		Short: "Install the Chrome native messaging host manifest",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := installNativeManifestForBrowser(extensionID, binaryPath, outputPath, browser)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
	cmd.Flags().StringVar(&extensionID, "extension-id", defaultChromeExtensionID, "Chrome extension id for allowed_origins")
	cmd.Flags().StringVar(&binaryPath, "path", "", "native host binary target for the stable host link")
	cmd.Flags().StringVar(&outputPath, "output", "", "native host manifest output path")
	cmd.Flags().StringVar(&browser, "browser", "", "browser to register (chrome, chrome-beta, bitbrowser, or BitBrowser instance id)")
	return cmd
}

func installNativeManifest(extensionID string, binaryPath string, outputPath string) (string, error) {
	return installNativeManifestForBrowser(extensionID, binaryPath, outputPath, "")
}

func installNativeManifestForBrowser(extensionID string, binaryPath string, outputPath string, browserSelector string) (string, error) {
	targetPath, err := resolveNativeHostTarget(binaryPath)
	if err != nil {
		return "", err
	}
	hostPath, err := stableNativeHostPath()
	if err != nil {
		return "", err
	}
	if err := installStableNativeHostLink(targetPath, hostPath); err != nil {
		return "", err
	}
	manifest, err := nativeManifest(extensionID, hostPath)
	if err != nil {
		return "", err
	}
	path := outputPath
	if path == "" {
		path, err = defaultNativeHostManifestPathForBrowser(browserSelector)
		if err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		return "", err
	}
	if outputPath == "" && runtime.GOOS == "windows" {
		key := `HKCU\Software\Google\Chrome\NativeMessagingHosts\` + host.NativeHostName
		if err := regAddDefaultString(key, path); err != nil {
			return "", fmt.Errorf("failed to register native messaging host %q: %w", key, err)
		}
	}
	return path, nil
}

type setupResult struct {
	NativeManifestPath    string
	ExternalExtensionPath string
	StoreURL              string
	OpenedStore           bool
	StoreOpenError        string
	SkillUpdate           skillUpdateStatus
}

type manualSetupResult struct {
	NativeManifestPath string
	ExtensionID        string
	ZIPPath            string
	UnpackedPath       string
	OpenedChrome       bool
	OpenedFileManager  bool
	SkillUpdate        skillUpdateStatus
}

type browserExtensionStatus struct {
	Installed       bool
	Reachable       bool
	ExtensionID     string
	Version         string
	VersionSource   string
	ExpectedVersion string
	InstallCommand  string
	UpgradeCommand  string
	Error           string
}

type skillUpdateStatus struct {
	Checked   bool
	Attempted bool
	Updated   bool
	Error     string
}

func setupChrome(extensionID string, binaryPath string, externalExtensionOutput string, browserSelector string) (setupResult, error) {
	if root, err := browserRootForInstallSelector(browserSelector); err == nil && root.BrowserID == "bitbrowser" {
		return setupResult{}, fmt.Errorf("BitBrowser does not support Chrome External Extensions setup; install the extension in BitBrowser, then run `open-browser-use install-manifest --browser %s`", bitBrowserInstallSelector(root))
	}
	manifestPath, err := installNativeManifestForBrowser(extensionID, binaryPath, "", browserSelector)
	if err != nil {
		return setupResult{}, err
	}
	extensionPath, err := installChromeExternalExtensionForBrowser(extensionID, externalExtensionOutput, browserSelector)
	if err != nil {
		return setupResult{}, err
	}
	return setupResult{
		NativeManifestPath:    manifestPath,
		ExternalExtensionPath: extensionPath,
		StoreURL:              chromeWebStoreExtensionURL,
		SkillUpdate:           maybeUpdateInstalledSkill(),
	}, nil
}

func bitBrowserInstallSelector(root browserProfileRoot) string {
	if root.BrowserInstance != "" {
		return root.BrowserInstance
	}
	return root.BrowserID
}

func renderStartupStatus(writer io.Writer) error {
	status := detectBrowserExtension(host.DefaultSocketDir, 700*time.Millisecond)
	fmt.Fprintln(writer, "Open Browser Use")
	fmt.Fprintf(writer, "📦 CLI version: %s\n", version)
	fmt.Fprintf(writer, "🧩 Browser extension: %s\n", status.summary())
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "Next steps:")
	if status.needsInstall() {
		fmt.Fprintf(writer, "  1. Install the browser extension: %s\n", status.InstallCommand)
		fmt.Fprintln(writer, "     If the Chrome Web Store item is unavailable, use: open-browser-use setup beta")
		fmt.Fprintln(writer, "  2. Restart Chrome if it asks you to enable the extension.")
		fmt.Fprintln(writer, "  3. Verify the connection: open-browser-use info")
		return nil
	}
	if status.needsUpgrade() {
		fmt.Fprintf(writer, "  1. Upgrade the browser extension: %s\n", status.UpgradeCommand)
		fmt.Fprintln(writer, "  2. Restart Chrome if it asks you to reload the extension.")
		fmt.Fprintln(writer, "  3. Verify the connection: open-browser-use info")
		return nil
	}
	if !status.Reachable {
		fmt.Fprintln(writer, "  1. Open Chrome and make sure the Open Browser Use extension is enabled.")
		fmt.Fprintln(writer, "  2. Verify the connection: open-browser-use info")
		return nil
	}
	fmt.Fprintln(writer, "  1. You are ready. Try: open-browser-use user-tabs")
	return nil
}

func renderStoreSetupResult(writer io.Writer, result setupResult, status browserExtensionStatus) error {
	fmt.Fprintln(writer, "✅ Open Browser Use setup")
	fmt.Fprintf(writer, "1. ✅ Registered native host\n   %s\n", result.NativeManifestPath)
	fmt.Fprintf(writer, "2. ✅ Requested Chrome extension install\n   Extension id: %s\n   Chrome install file: %s\n", defaultChromeExtensionID, result.ExternalExtensionPath)
	if result.OpenedStore {
		fmt.Fprintf(writer, "3. ✅ Opened Chrome Web Store\n   %s\n", result.StoreURL)
	} else if result.StoreOpenError != "" {
		fmt.Fprintf(writer, "3. ⚠️ Chrome Web Store page was not opened\n   %s\n   Error: %s\n", result.StoreURL, result.StoreOpenError)
	} else {
		fmt.Fprintf(writer, "3. Open Chrome Web Store\n   %s\n", result.StoreURL)
	}
	fmt.Fprintf(writer, "4. 🧩 Browser extension\n   %s\n", status.summaryForSetup("Not installed yet. Install or enable it from the Chrome Web Store page."))
	renderSkillUpdateResult(writer, result.SkillUpdate)
	fmt.Fprintln(writer)
	if status.isReady() {
		fmt.Fprintln(writer, "All set. Browser extension is installed, connected, and on the expected version.")
		return nil
	}
	if status.needsUpgrade() {
		fmt.Fprintf(writer, "Next: upgrade the browser extension with `%s`, then run `open-browser-use info`.\n", status.UpgradeCommand)
		return nil
	}
	if result.OpenedStore {
		fmt.Fprintln(writer, "Next: the Chrome Web Store page is open; install or enable the extension, then run `open-browser-use info`.")
		return nil
	}
	if result.StoreOpenError != "" {
		fmt.Fprintln(writer, "Next: open the Chrome Web Store page manually, install or enable the extension, then run `open-browser-use info`.")
		return nil
	}
	fmt.Fprintln(writer, "Next: restart Chrome if needed, approve or enable the extension if Chrome asks, then run `open-browser-use info`.")
	return nil
}

func renderManualSetupResult(writer io.Writer, result manualSetupResult, status browserExtensionStatus) error {
	fmt.Fprintln(writer, "✅ Open Browser Use beta setup")
	fmt.Fprintf(writer, "1. ✅ Registered native host\n   %s\n", result.NativeManifestPath)
	fmt.Fprintf(writer, "2. ✅ Prepared browser extension package\n   Extension id: %s\n   ZIP: %s\n   Includes stable extension key for this id.\n", result.ExtensionID, result.ZIPPath)
	fmt.Fprintf(writer, "3. ✅ Prepared unpacked extension directory\n   %s\n", result.UnpackedPath)
	fmt.Fprintf(writer, "4. 🧩 Browser extension\n   %s\n", status.summaryForSetup("Not installed yet. Finish the ZIP install below."))
	renderSkillUpdateResult(writer, result.SkillUpdate)
	fmt.Fprintln(writer)
	if status.isReady() {
		fmt.Fprintln(writer, "All set. Browser extension is installed, connected, and on the expected version.")
		return nil
	}
	if result.OpenedChrome && result.OpenedFileManager {
		fmt.Fprintln(writer, "Next: chrome://extensions and Finder/file manager are open; enable Developer mode, drag the ZIP into Chrome, then run `open-browser-use info`.")
		return nil
	}
	fmt.Fprintf(writer, "Next: open chrome://extensions, enable Developer mode, drag in %s, then run `open-browser-use info`.\n", result.ZIPPath)
	return nil
}

func renderSkillUpdateResult(writer io.Writer, status skillUpdateStatus) {
	if status.Updated {
		fmt.Fprintln(writer, "5. ✅ Agent skill\n   Updated existing open-browser-use skill.")
		return
	}
	if status.Attempted && status.Error != "" {
		fmt.Fprintf(writer, "5. ⚠️ Agent skill\n   Existing skill update failed: %s\n", status.Error)
		return
	}
	if !status.Checked {
		return
	}
	fmt.Fprintln(writer, "5. ℹ️ Agent skill install commands\n   Codex: npx skills add iFurySt/open-browser-use -g -a codex --skill open-browser-use --copy -y\n   Claude Code: npx skills add iFurySt/open-browser-use -g -a claude-code --skill open-browser-use --copy -y")
}

func detectBrowserExtension(socketDir string, timeout time.Duration) browserExtensionStatus {
	return detectBrowserExtensionForBrowser(socketDir, timeout, "")
}

func detectBrowserExtensionForBrowser(socketDir string, timeout time.Duration, browserSelector string) browserExtensionStatus {
	status := browserExtensionStatus{
		ExpectedVersion: version,
		InstallCommand:  "open-browser-use setup",
		UpgradeCommand:  "open-browser-use setup",
	}
	if strings.TrimSpace(browserSelector) != "" {
		status.InstallCommand = "open-browser-use setup --browser " + browserSelector
		status.UpgradeCommand = status.InstallCommand
	}
	if response, err := invokeWithProfile("", socketDir, browserSelector, "", "getInfo", map[string]any{}, timeout); err == nil {
		if result, ok := response["result"].(map[string]any); ok {
			status.Installed = true
			status.Reachable = true
			status.VersionSource = "connected extension"
			if extensionVersion, ok := result["version"].(string); ok {
				status.Version = extensionVersion
			}
			if metadata, ok := result["metadata"].(map[string]any); ok {
				if extensionID, ok := metadata["extensionId"].(string); ok {
					status.ExtensionID = extensionID
				}
			}
			return status
		}
	} else {
		status.Error = err.Error()
	}

	if detected, ok := detectInstalledChromeExtensionForBrowser(browserSelector); ok {
		status.Installed = true
		status.ExtensionID = detected.ExtensionID
		status.Version = detected.Version
		status.VersionSource = detected.Source
	}
	return status
}

func detectInstalledChromeExtensionForBrowser(browserSelector string) (detectedExtension, bool) {
	if strings.TrimSpace(browserSelector) == "" {
		return detectInstalledChromeExtension()
	}
	profiles, err := listInstalledChromeProfiles()
	if err != nil {
		return detectedExtension{}, false
	}
	var best detectedExtension
	for _, profile := range profiles {
		info := connectedProfileInfo{
			Browser:         profile.Browser,
			BrowserName:     profile.BrowserName,
			BrowserInstance: profile.BrowserInstance,
			Target:          profile.Target,
		}
		if !browserSelectorMatches(browserSelector, info) {
			continue
		}
		detected := detectedExtension{
			ExtensionID: profile.ExtensionID,
			Version:     profile.Version,
			Source:      profile.Source,
		}
		if best.Version == "" || compareChromeVersions(detected.Version, best.Version) > 0 {
			best = detected
		}
	}
	return best, best.Version != ""
}

type detectedExtension struct {
	ExtensionID string
	Version     string
	Source      string
}

type installedChromeProfile struct {
	Browser         string `json:"browser,omitempty"`
	BrowserName     string `json:"browserName,omitempty"`
	BrowserInstance string `json:"browserInstance,omitempty"`
	Target          string `json:"target,omitempty"`
	Directory       string `json:"directory"`
	DisplayName     string `json:"displayName,omitempty"`
	ExtensionID     string `json:"extensionId"`
	Version         string `json:"version"`
	Source          string `json:"source"`
}

func chromeExtensionCandidateIDs() []string {
	candidates := []string{defaultChromeExtensionID}
	if betaID, err := extensionIDFromPublicKey(betaExtensionPublicKey); err == nil && betaID != defaultChromeExtensionID {
		candidates = append(candidates, betaID)
	}
	return candidates
}

func detectInstalledChromeExtension() (detectedExtension, bool) {
	var best detectedExtension
	for _, extensionID := range chromeExtensionCandidateIDs() {
		detected, ok := detectInstalledChromeExtensionByID(extensionID)
		if !ok {
			continue
		}
		if best.Version == "" || compareChromeVersions(detected.Version, best.Version) > 0 {
			best = detected
		}
	}
	return best, best.Version != ""
}

func detectInstalledChromeExtensionByID(extensionID string) (detectedExtension, bool) {
	roots, err := supportedBrowserProfileRoots()
	if err != nil {
		return detectedExtension{}, false
	}
	var best detectedExtension
	for _, root := range roots {
		profiles, err := chromeProfileDirs(root.Root)
		if err != nil {
			continue
		}
		for _, profile := range profiles {
			detected, ok := detectExtensionInProfile(profile, extensionID)
			if !ok {
				continue
			}
			if best.Version == "" || compareChromeVersions(detected.Version, best.Version) > 0 {
				best = detected
			}
		}
	}
	return best, best.Version != ""
}

func detectExtensionInProfile(profileDir string, extensionID string) (detectedExtension, bool) {
	var best detectedExtension
	if detected, ok := detectCRXExtensionInProfile(profileDir, extensionID); ok {
		if best.Version == "" || compareChromeVersions(detected.Version, best.Version) > 0 {
			best = detected
		}
	}
	if detected, ok := detectUnpackedExtensionInProfile(profileDir, extensionID); ok {
		if best.Version == "" || compareChromeVersions(detected.Version, best.Version) > 0 {
			best = detected
		}
	}
	return best, best.Version != ""
}

func detectCRXExtensionInProfile(profileDir string, extensionID string) (detectedExtension, bool) {
	versionDirs, err := filepath.Glob(filepath.Join(profileDir, "Extensions", extensionID, "*"))
	if err != nil {
		return detectedExtension{}, false
	}
	var best detectedExtension
	for _, versionDir := range versionDirs {
		manifestPath := filepath.Join(versionDir, "manifest.json")
		detected, ok := readManifestVersion(manifestPath, extensionID)
		if !ok {
			continue
		}
		if best.Version == "" || compareChromeVersions(detected.Version, best.Version) > 0 {
			best = detected
		}
	}
	return best, best.Version != ""
}

func detectUnpackedExtensionInProfile(profileDir string, extensionID string) (detectedExtension, bool) {
	securePrefsPath := filepath.Join(profileDir, "Secure Preferences")
	payload, err := os.ReadFile(securePrefsPath)
	if err != nil {
		return detectedExtension{}, false
	}
	var prefs struct {
		Extensions struct {
			Settings map[string]struct {
				Path     string         `json:"path"`
				Manifest map[string]any `json:"manifest"`
			} `json:"settings"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(payload, &prefs); err != nil {
		return detectedExtension{}, false
	}
	entry, ok := prefs.Extensions.Settings[extensionID]
	if !ok || strings.TrimSpace(entry.Path) == "" {
		return detectedExtension{}, false
	}
	resolved := entry.Path
	if !filepath.IsAbs(resolved) {
		// CRX-style paths in Secure Preferences are usually `<id>/<version>`
		// rooted at `<profile>/Extensions/`.
		resolved = filepath.Join(profileDir, "Extensions", resolved)
	}
	manifestPath := filepath.Join(resolved, "manifest.json")
	if detected, ok := readManifestVersion(manifestPath, extensionID); ok {
		return detected, true
	}
	// Fall back to the manifest blob embedded in Secure Preferences if the
	// on-disk manifest is unreachable.
	if version, ok := entry.Manifest["version"].(string); ok && strings.TrimSpace(version) != "" {
		return detectedExtension{
			ExtensionID: extensionID,
			Version:     strings.TrimSpace(version),
			Source:      securePrefsPath,
		}, true
	}
	return detectedExtension{}, false
}

func readManifestVersion(manifestPath string, extensionID string) (detectedExtension, bool) {
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		return detectedExtension{}, false
	}
	var manifest map[string]any
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return detectedExtension{}, false
	}
	extensionVersion, ok := manifest["version"].(string)
	if !ok || strings.TrimSpace(extensionVersion) == "" {
		return detectedExtension{}, false
	}
	return detectedExtension{
		ExtensionID: extensionID,
		Version:     strings.TrimSpace(extensionVersion),
		Source:      manifestPath,
	}, true
}

func listInstalledChromeProfiles() ([]installedChromeProfile, error) {
	roots, err := supportedBrowserProfileRoots()
	if err != nil {
		return nil, err
	}
	candidates := chromeExtensionCandidateIDs()

	var profiles []installedChromeProfile
	for _, root := range roots {
		profileDirs, err := chromeProfileDirs(root.Root)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		displayNames := chromeProfileDisplayNames(root.Root)
		for _, profileDir := range profileDirs {
			var best detectedExtension
			for _, extensionID := range candidates {
				detected, ok := detectExtensionInProfile(profileDir, extensionID)
				if !ok {
					continue
				}
				if best.Version == "" || compareChromeVersions(detected.Version, best.Version) > 0 {
					best = detected
				}
			}
			if best.Version == "" {
				continue
			}
			dirName := filepath.Base(profileDir)
			profiles = append(profiles, installedChromeProfile{
				Browser:         root.BrowserID,
				BrowserName:     root.BrowserName,
				BrowserInstance: root.BrowserInstance,
				Target:          browserProfileTarget(root.BrowserID, root.BrowserInstance, dirName),
				Directory:       dirName,
				DisplayName:     displayNames[dirName],
				ExtensionID:     best.ExtensionID,
				Version:         best.Version,
				Source:          best.Source,
			})
		}
	}
	sort.Slice(profiles, func(i, j int) bool {
		left := profiles[i].Browser + ":" + profiles[i].BrowserInstance + ":" + chromeProfileSortKey(profiles[i].Directory)
		right := profiles[j].Browser + ":" + profiles[j].BrowserInstance + ":" + chromeProfileSortKey(profiles[j].Directory)
		return left < right
	})
	return profiles, nil
}

func browserProfileTarget(browserID string, browserInstance string, directory string) string {
	parts := []string{browserID}
	if browserInstance != "" {
		parts = append(parts, browserInstance)
	}
	if directory != "" {
		parts = append(parts, directory)
	}
	return strings.Join(parts, ":")
}

func chromeProfileSortKey(dir string) string {
	// Keep "Default" first, then "Profile N" ordered numerically.
	if dir == "Default" {
		return "0"
	}
	if strings.HasPrefix(dir, "Profile ") {
		suffix := strings.TrimPrefix(dir, "Profile ")
		if n, err := strconv.Atoi(suffix); err == nil {
			return fmt.Sprintf("1:%010d", n)
		}
	}
	return "2:" + dir
}

// resolveProfileForInstanceID searches each Chrome profile's
// `Local Extension Settings/<extensionID>/` LevelDB files for the given
// instanceID UUID. LevelDB stores values as raw bytes, so a UUID string written
// via `chrome.storage.local` appears verbatim in the .log / .ldb files of the
// profile that generated it. This gives us a stable mapping from
// instanceID -> profile directory without needing the host to know its profile
// at startup or requiring extra extension permissions.
func resolveProfileForInstanceID(extensionID string, instanceID string) (browserID string, browserName string, browserInstance string, directory string, displayName string, ok bool) {
	if extensionID == "" || instanceID == "" {
		return "", "", "", "", "", false
	}
	roots, err := supportedBrowserProfileRoots()
	if err != nil {
		return "", "", "", "", "", false
	}
	needle := []byte(instanceID)
	for _, root := range roots {
		profiles, err := chromeProfileDirs(root.Root)
		if err != nil {
			continue
		}
		names := chromeProfileDisplayNames(root.Root)
		for _, profileDir := range profiles {
			storageDir := filepath.Join(profileDir, "Local Extension Settings", extensionID)
			entries, err := os.ReadDir(storageDir)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				if !strings.HasSuffix(name, ".log") && !strings.HasSuffix(name, ".ldb") {
					continue
				}
				info, err := entry.Info()
				if err != nil || info.Size() > maxLevelDBScanBytes {
					continue
				}
				payload, err := os.ReadFile(filepath.Join(storageDir, name))
				if err != nil {
					continue
				}
				if !bytes.Contains(payload, needle) {
					continue
				}
				dir := filepath.Base(profileDir)
				return root.BrowserID, root.BrowserName, root.BrowserInstance, dir, names[dir], true
			}
		}
	}
	return "", "", "", "", "", false
}

func chromeProfileDisplayNames(root string) map[string]string {
	payload, err := os.ReadFile(filepath.Join(root, "Local State"))
	if err != nil {
		return nil
	}
	var state struct {
		Profile struct {
			InfoCache map[string]struct {
				Name string `json:"name"`
			} `json:"info_cache"`
		} `json:"profile"`
	}
	if err := json.Unmarshal(payload, &state); err != nil {
		return nil
	}
	out := make(map[string]string, len(state.Profile.InfoCache))
	for dir, info := range state.Profile.InfoCache {
		if name := strings.TrimSpace(info.Name); name != "" {
			out[dir] = name
		}
	}
	return out
}

func defaultChromeUserDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library/Application Support/Google/Chrome"), nil
	case "linux":
		return filepath.Join(home, ".config/google-chrome"), nil
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if strings.TrimSpace(localAppData) == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "Google", "Chrome", "User Data"), nil
	default:
		return "", fmt.Errorf("Chrome extension detection is not implemented for %s", runtime.GOOS)
	}
}

func chromeProfileDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	profiles := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "Default" || strings.HasPrefix(name, "Profile ") {
			profiles = append(profiles, filepath.Join(root, name))
		}
	}
	return profiles, nil
}

func (status browserExtensionStatus) summary() string {
	if status.isReady() {
		return fmt.Sprintf("Ready, installed and connected at v%s.", status.Version)
	}
	if status.needsUpgrade() {
		return fmt.Sprintf("Installed v%s, but CLI expects v%s. Run `%s` to upgrade.", status.Version, status.ExpectedVersion, status.UpgradeCommand)
	}
	if status.Installed {
		if status.Version != "" {
			if compareChromeVersions(status.Version, status.ExpectedVersion) >= 0 {
				return fmt.Sprintf("Installed v%s and version matches CLI v%s. Open Chrome and run `open-browser-use info` to verify the connection.", status.Version, status.ExpectedVersion)
			}
			return fmt.Sprintf("Installed v%s, but not connected yet. Open Chrome and run `open-browser-use info`.", status.Version)
		}
		return "Installed, but not connected yet. Open Chrome and run `open-browser-use info`."
	}
	return fmt.Sprintf("Not installed yet. Run `%s`.", status.InstallCommand)
}

func (status browserExtensionStatus) summaryForSetup(missingMessage string) string {
	if status.needsInstall() {
		return missingMessage
	}
	return status.summary()
}

func (status browserExtensionStatus) isReady() bool {
	return status.Reachable && status.Installed && compareChromeVersions(status.Version, status.ExpectedVersion) >= 0
}

func (status browserExtensionStatus) needsInstall() bool {
	return !status.Installed
}

func (status browserExtensionStatus) needsUpgrade() bool {
	return status.Installed && status.Version != "" && compareChromeVersions(status.Version, status.ExpectedVersion) < 0
}

func shouldOpenManualSetup(status browserExtensionStatus, noOpen bool) bool {
	if noOpen {
		return false
	}
	return status.needsInstall() || status.needsUpgrade()
}

func shouldOpenStoreSetup(status browserExtensionStatus, noOpen bool) bool {
	if noOpen {
		return false
	}
	return status.needsInstall() || status.needsUpgrade()
}

func maybeUpdateInstalledSkill() skillUpdateStatus {
	npxPath, err := exec.LookPath("npx")
	if err != nil {
		return skillUpdateStatus{}
	}
	status := skillUpdateStatus{Checked: true}
	if err := runSilentCommand(20*time.Second, npxPath, "skills"); err != nil {
		return status
	}
	status.Attempted = true
	if err := runSilentCommand(2*time.Minute, npxPath, "skills", "update", "open-browser-use", "-g", "-y"); err != nil {
		status.Error = err.Error()
		return status
	}
	status.Updated = true
	return status
}

func runSilentCommand(timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("%s timed out", strings.Join(append([]string{name}, args...), " "))
	}
	return err
}

func compareChromeVersions(left string, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	maxParts := len(leftParts)
	if len(rightParts) > maxParts {
		maxParts = len(rightParts)
	}
	for i := 0; i < maxParts; i++ {
		leftValue := 0
		rightValue := 0
		if i < len(leftParts) {
			_, _ = fmt.Sscanf(leftParts[i], "%d", &leftValue)
		}
		if i < len(rightParts) {
			_, _ = fmt.Sscanf(rightParts[i], "%d", &rightValue)
		}
		if leftValue > rightValue {
			return 1
		}
		if leftValue < rightValue {
			return -1
		}
	}
	return 0
}

func installChromeExternalExtension(extensionID string, outputPath string) (string, error) {
	return installChromeExternalExtensionForBrowser(extensionID, outputPath, "")
}

func installChromeExternalExtensionForBrowser(extensionID string, outputPath string, browserSelector string) (string, error) {
	allowedExtensionID := strings.TrimSpace(extensionID)
	if allowedExtensionID == "" {
		allowedExtensionID = defaultChromeExtensionID
	}
	if runtime.GOOS == "windows" && outputPath == "" {
		key := `HKCU\Software\Google\Chrome\Extensions\` + allowedExtensionID
		if err := regAddString(key, "update_url", chromeWebStoreUpdateURL); err != nil {
			return "", fmt.Errorf("failed to register Chrome external extension %q: %w", key, err)
		}
		return "registry:" + key, nil
	}
	path := outputPath
	if path == "" {
		var err error
		path, err = defaultChromeExternalExtensionPathForBrowser(allowedExtensionID, browserSelector)
		if err != nil {
			return "", err
		}
	}
	manifest := map[string]any{
		"external_update_url": chromeWebStoreUpdateURL,
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("failed to create Chrome external extension directory %q: %w", filepath.Dir(path), err)
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("failed to write Chrome external extension manifest %q: %w", path, err)
	}
	return path, nil
}

type socketOptions struct {
	socketPath string
	socketDir  string
	timeout    time.Duration
	sessionID  string
	browser    string
	profile    string
}

func addSocketFlags(cmd *cobra.Command, options *socketOptions) {
	if options.sessionID == "" {
		options.sessionID = defaultCLISessionID
	}
	cmd.Flags().StringVar(&options.socketPath, "socket", "", "open-browser-use Unix socket path")
	cmd.Flags().StringVar(&options.socketDir, "socket-dir", host.DefaultSocketDir, "directory containing active socket registry")
	cmd.Flags().DurationVar(&options.timeout, "timeout", 10*time.Second, "request timeout")
	cmd.Flags().StringVar(&options.sessionID, "session-id", options.sessionID, "browser session id used for tab grouping and cleanup")
	cmd.Flags().StringVar(&options.browser, "browser", "", "browser selector (chrome, chrome-beta, bitbrowser, or display name)")
	cmd.Flags().StringVar(&options.profile, "profile", "", "Chrome profile selector (directory name like \"Default\" / \"Profile 1\" or display name like \"Eva\")")
}

func newCallCommand() *cobra.Command {
	var options socketOptions
	var method string
	var params string
	cmd := &cobra.Command{
		Use:   "call",
		Short: "Send an unrestricted JSON-RPC request",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if method == "" {
				return errors.New("call requires --method")
			}
			var paramValue map[string]any
			if err := json.Unmarshal([]byte(params), &paramValue); err != nil {
				return err
			}
			return invokeAndWrite(options, method, paramValue)
		},
	}
	addSocketFlags(cmd, &options)
	cmd.Flags().StringVar(&method, "method", "", "JSON-RPC method")
	cmd.Flags().StringVar(&params, "params", "{}", "JSON object params")
	return cmd
}

func newRunCommand() *cobra.Command {
	var options socketOptions
	var command string
	var file string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a line-oriented browser action plan",
		Long: `Run a line-oriented browser action plan.

The plan is not a general programming language. Each non-empty, non-comment line
is one supported browser action. Actions share the same session and turn, and
open-tab or claim-tab sets the default tab for later tab-scoped actions.

Example:
  open-browser-use run -c '
  name-session "Docs scan - OBU"
  open-tab https://docs.browser-use.com
  wait-load domcontentloaded
  page-info
  finalize-tabs []
  '`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if command != "" && file != "" {
				return errors.New("run accepts either --command or --file, not both")
			}
			script := command
			if file != "" {
				payload, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				script = string(payload)
			}
			if strings.TrimSpace(script) == "" {
				return errors.New("run requires --command or --file")
			}
			runner := newActionRunner(options)
			output, err := runner.run(script)
			if err != nil {
				return err
			}
			return writeJSONTo(cmd.OutOrStdout(), output)
		},
	}
	addSocketFlags(cmd, &options)
	cmd.Flags().StringVarP(&command, "command", "c", "", "line-oriented action plan")
	cmd.Flags().StringVar(&file, "file", "", "file containing a line-oriented action plan")
	return cmd
}

func newSimpleRPCCommand(use string, method string, short string) *cobra.Command {
	var options socketOptions
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return invokeAndWrite(options, method, map[string]any{})
		},
	}
	addSocketFlags(cmd, &options)
	return cmd
}

func newHistoryCommand() *cobra.Command {
	var options socketOptions
	var query string
	var limit int
	var from string
	var to string
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Search Chrome history",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			params := map[string]any{"query": query, "limit": limit}
			if from != "" {
				params["from"] = from
			}
			if to != "" {
				params["to"] = to
			}
			return invokeAndWrite(options, "getUserHistory", params)
		},
	}
	addSocketFlags(cmd, &options)
	cmd.Flags().StringVar(&query, "query", "", "history search query")
	cmd.Flags().IntVar(&limit, "limit", 100, "maximum result count")
	cmd.Flags().StringVar(&from, "from", "", "optional ISO start date")
	cmd.Flags().StringVar(&to, "to", "", "optional ISO end date")
	return cmd
}

func newClaimTabCommand() *cobra.Command {
	var options socketOptions
	var tabID int
	cmd := &cobra.Command{
		Use:   "claim-tab",
		Short: "Claim an existing Chrome tab",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if tabID <= 0 {
				return errors.New("claim-tab requires --tab-id")
			}
			return invokeAndWrite(options, "claimUserTab", map[string]any{"tabId": tabID})
		},
	}
	addSocketFlags(cmd, &options)
	cmd.Flags().IntVar(&tabID, "tab-id", 0, "Chrome tab id to claim")
	return cmd
}

func newFinalizeTabsCommand() *cobra.Command {
	var options socketOptions
	var keepJSON string
	cmd := &cobra.Command{
		Use:   "finalize-tabs",
		Short: "Finalize session tabs",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			var keep []any
			if err := json.Unmarshal([]byte(keepJSON), &keep); err != nil {
				return fmt.Errorf("finalize-tabs --keep must be a JSON array: %w", err)
			}
			return invokeAndWrite(options, "finalizeTabs", map[string]any{"keep": keep})
		},
	}
	addSocketFlags(cmd, &options)
	cmd.Flags().StringVar(&keepJSON, "keep", "[]", `tabs to keep, e.g. '[{"tabId":123,"status":"handoff"}]'`)
	return cmd
}

func newNameSessionCommand() *cobra.Command {
	var options socketOptions
	var name string
	cmd := &cobra.Command{
		Use:   "name-session",
		Short: "Set the session tab group name",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if name == "" {
				return errors.New("name-session requires --name")
			}
			return invokeAndWrite(options, "nameSession", map[string]any{"name": name})
		},
	}
	addSocketFlags(cmd, &options)
	cmd.Flags().StringVar(&name, "name", "", "session group title")
	return cmd
}

func newCdpCommand() *cobra.Command {
	var options socketOptions
	var tabID int
	var method string
	var commandParamsJSON string
	cmd := &cobra.Command{
		Use:   "cdp",
		Short: "Run a Chrome DevTools Protocol command",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if method == "" {
				return errors.New("cdp requires --method")
			}
			var commandParams map[string]any
			if err := json.Unmarshal([]byte(commandParamsJSON), &commandParams); err != nil {
				return fmt.Errorf("cdp --params must be a JSON object: %w", err)
			}
			target := map[string]any{}
			if tabID > 0 {
				target["tabId"] = tabID
				_, _ = invokeWithOptions(options, "attach", map[string]any{"tabId": tabID})
			}
			return invokeAndWrite(options, "executeCdp", map[string]any{
				"target":        target,
				"method":        method,
				"commandParams": commandParams,
			})
		},
	}
	addSocketFlags(cmd, &options)
	cmd.Flags().IntVar(&tabID, "tab-id", 0, "optional tab id target")
	cmd.Flags().StringVar(&method, "method", "", "Chrome DevTools Protocol method")
	cmd.Flags().StringVar(&commandParamsJSON, "params", "{}", "Chrome DevTools Protocol command params")
	return cmd
}

func newMoveMouseCommand() *cobra.Command {
	var options socketOptions
	var tabID int
	var x float64
	var y float64
	var wait bool
	cmd := &cobra.Command{
		Use:   "move-mouse",
		Short: "Move the browser cursor overlay",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if tabID <= 0 {
				return errors.New("move-mouse requires --tab-id")
			}
			return invokeAndWrite(options, "moveMouse", map[string]any{
				"tabId":          tabID,
				"x":              x,
				"y":              y,
				"waitForArrival": wait,
			})
		},
	}
	addSocketFlags(cmd, &options)
	cmd.Flags().IntVar(&tabID, "tab-id", 0, "tab id to move the cursor in")
	cmd.Flags().Float64Var(&x, "x", 0, "viewport x coordinate")
	cmd.Flags().Float64Var(&y, "y", 0, "viewport y coordinate")
	cmd.Flags().BoolVar(&wait, "wait", true, "wait for cursor arrival acknowledgement")
	return cmd
}

func newWaitFileChooserCommand() *cobra.Command {
	var options socketOptions
	var tabID int
	cmd := &cobra.Command{
		Use:   "wait-file-chooser",
		Short: "Wait for a browser session tab to open a file chooser",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if tabID <= 0 {
				return errors.New("wait-file-chooser requires --tab-id")
			}
			return invokeAndWrite(options, "waitForFileChooser", map[string]any{
				"tabId":     tabID,
				"timeoutMs": int(options.timeout.Milliseconds()),
			})
		},
	}
	addSocketFlags(cmd, &options)
	cmd.Flags().IntVar(&tabID, "tab-id", 0, "tab id expected to open a file chooser")
	return cmd
}

func newSetFileChooserFilesCommand() *cobra.Command {
	var options socketOptions
	var fileChooserID string
	var files []string
	cmd := &cobra.Command{
		Use:   "set-file-chooser-files",
		Short: "Set files for an intercepted browser file chooser",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if fileChooserID == "" {
				return errors.New("set-file-chooser-files requires --file-chooser-id")
			}
			if len(files) == 0 {
				return errors.New("set-file-chooser-files requires at least one --file")
			}
			return invokeAndWrite(options, "setFileChooserFiles", map[string]any{
				"fileChooserId": fileChooserID,
				"files":         files,
			})
		},
	}
	addSocketFlags(cmd, &options)
	cmd.Flags().StringVar(&fileChooserID, "file-chooser-id", "", "file chooser id returned by wait-file-chooser")
	cmd.Flags().StringSliceVar(&files, "file", nil, "file path to set; repeat or comma-separate for multiple files")
	return cmd
}

func newOpenTabCommand() *cobra.Command {
	var options socketOptions
	var url string
	cmd := &cobra.Command{
		Use:   "open-tab",
		Short: "Open a new browser session tab",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			tabResponse, err := invokeWithOptions(options, "createTab", map[string]any{})
			if err != nil {
				return err
			}
			result, _ := tabResponse["result"].(map[string]any)
			tabID, ok := numberAsInt(result["id"])
			if !ok {
				return errors.New("createTab response did not include numeric tab id")
			}
			output := map[string]any{"tab": result}
			if url != "" {
				_, _ = invokeWithOptions(options, "attach", map[string]any{"tabId": tabID})
				navigateResponse, err := invokeWithOptions(options, "executeCdp", map[string]any{
					"target":        map[string]any{"tabId": tabID},
					"method":        "Page.navigate",
					"commandParams": map[string]any{"url": url},
				})
				if err != nil {
					return err
				}
				output["navigate"] = navigateResponse["result"]
			}
			return writeJSON(output)
		},
	}
	addSocketFlags(cmd, &options)
	cmd.Flags().StringVar(&url, "url", "", "optional URL to navigate after tab creation")
	return cmd
}

func newNavigateCommand() *cobra.Command {
	var options socketOptions
	var tabID int
	var url string
	cmd := &cobra.Command{
		Use:   "navigate",
		Short: "Navigate a browser session tab",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if tabID <= 0 {
				return errors.New("navigate requires --tab-id")
			}
			if url == "" {
				return errors.New("navigate requires --url")
			}
			_, _ = invokeWithOptions(options, "attach", map[string]any{"tabId": tabID})
			return invokeAndWrite(options, "executeCdp", map[string]any{
				"target":        map[string]any{"tabId": tabID},
				"method":        "Page.navigate",
				"commandParams": map[string]any{"url": url},
			})
		},
	}
	addSocketFlags(cmd, &options)
	cmd.Flags().IntVar(&tabID, "tab-id", 0, "tab id to navigate")
	cmd.Flags().StringVar(&url, "url", "", "URL to navigate to")
	return cmd
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), version)
			return nil
		},
	}
}

func newProfilesCommand() *cobra.Command {
	var asJSON bool
	var connected bool
	var socketDir string
	cmd := &cobra.Command{
		Use:   "profiles",
		Short: "List Chrome profiles that have the Open Browser Use extension installed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			profiles, err := listInstalledChromeProfiles()
			if err != nil {
				return err
			}
			var connections []connectedProfileInfo
			if connected {
				connections = probeConnectedProfiles(socketDir)
			}
			return renderProfilesList(cmd.OutOrStdout(), profiles, connections, connected, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON instead of a human-readable table")
	cmd.Flags().BoolVar(&connected, "connected", false, "also probe running hosts and mark which profile is currently connected")
	cmd.Flags().StringVar(&socketDir, "socket-dir", host.DefaultSocketDir, "directory containing host sockets (used with --connected)")
	return cmd
}

const connectedProbeTimeout = 800 * time.Millisecond

type browserDefinition struct {
	ID          string
	Name        string
	UserDataDir string
}

type browserProfileRoot struct {
	BrowserID       string
	BrowserName     string
	BrowserInstance string
	Root            string
}

func supportedBrowserProfileRoots() ([]browserProfileRoot, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	switch runtime.GOOS {
	case "darwin":
		roots := []browserProfileRoot{
			{
				BrowserID:   "chrome",
				BrowserName: "Google Chrome",
				Root:        filepath.Join(home, "Library/Application Support/Google/Chrome"),
			},
			{
				BrowserID:   "chrome-beta",
				BrowserName: "Google Chrome Beta",
				Root:        filepath.Join(home, "Library/Application Support/Google/Chrome Beta"),
			},
		}
		bitBrowserRoots, err := bitBrowserProfileRoots(home)
		if err != nil {
			return nil, err
		}
		roots = append(roots, bitBrowserRoots...)
		return roots, nil
	case "linux":
		return []browserProfileRoot{{
			BrowserID:   "chrome",
			BrowserName: "Google Chrome",
			Root:        filepath.Join(home, ".config/google-chrome"),
		}}, nil
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if strings.TrimSpace(localAppData) == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return []browserProfileRoot{
			{
				BrowserID:   "chrome",
				BrowserName: "Google Chrome",
				Root:        filepath.Join(localAppData, "Google", "Chrome", "User Data"),
			},
			{
				BrowserID:   "chrome-beta",
				BrowserName: "Google Chrome Beta",
				Root:        filepath.Join(localAppData, "Google", "Chrome Beta", "User Data"),
			},
		}, nil
	default:
		return nil, fmt.Errorf("browser profile detection is not implemented for %s", runtime.GOOS)
	}
}

func bitBrowserProfileRoots(home string) ([]browserProfileRoot, error) {
	cacheRoot := filepath.Join(home, "Library/Application Support/BitBrowser/BrowserCache")
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var roots []browserProfileRoot
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		root := filepath.Join(cacheRoot, entry.Name())
		if _, err := os.Stat(filepath.Join(root, "Local State")); err != nil {
			continue
		}
		roots = append(roots, browserProfileRoot{
			BrowserID:       "bitbrowser",
			BrowserName:     "BitBrowser",
			BrowserInstance: entry.Name(),
			Root:            root,
		})
	}
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].BrowserInstance < roots[j].BrowserInstance
	})
	return roots, nil
}

func probeConnectedProfiles(socketDir string) []connectedProfileInfo {
	candidates, err := listSocketCandidates(socketDir)
	if err != nil || len(candidates) == 0 {
		return nil
	}
	probeTimeout := connectedProbeTimeout
	var out []connectedProfileInfo
	for _, candidate := range candidates {
		conn, err := net.DialTimeout("unix", candidate.path, probeTimeout)
		if err != nil {
			continue
		}
		resp, err := getInfoOverConn(conn, probeTimeout)
		_ = conn.Close()
		if err != nil {
			continue
		}
		out = append(out, connectedProfileFromInfo(candidate.path, resp))
	}
	return out
}

func renderProfilesList(writer io.Writer, profiles []installedChromeProfile, connected []connectedProfileInfo, showConnected bool, asJSON bool) error {
	connectedByTarget := map[string]connectedProfileInfo{}
	for _, info := range connected {
		if info.Target != "" {
			connectedByTarget[info.Target] = info
		}
	}

	if asJSON {
		type row struct {
			Browser         string `json:"browser,omitempty"`
			BrowserName     string `json:"browserName,omitempty"`
			BrowserInstance string `json:"browserInstance,omitempty"`
			Target          string `json:"target,omitempty"`
			Directory       string `json:"directory"`
			DisplayName     string `json:"displayName,omitempty"`
			ExtensionID     string `json:"extensionId"`
			Version         string `json:"version"`
			Source          string `json:"source"`
			Connected       bool   `json:"connected,omitempty"`
			SocketPath      string `json:"socketPath,omitempty"`
			InstanceID      string `json:"instanceId,omitempty"`
		}
		rows := make([]row, 0, len(profiles))
		for _, profile := range profiles {
			r := row{
				Browser:         profile.Browser,
				BrowserName:     profile.BrowserName,
				BrowserInstance: profile.BrowserInstance,
				Target:          profile.Target,
				Directory:       profile.Directory,
				DisplayName:     profile.DisplayName,
				ExtensionID:     profile.ExtensionID,
				Version:         profile.Version,
				Source:          profile.Source,
			}
			if showConnected {
				if info, ok := connectedByTarget[profile.Target]; ok {
					r.Connected = true
					r.SocketPath = info.SocketPath
					r.InstanceID = info.InstanceID
				}
			}
			rows = append(rows, r)
		}
		payload, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(writer, string(payload))
		return nil
	}
	if len(profiles) == 0 {
		fmt.Fprintln(writer, "No browser profiles with the Open Browser Use extension were detected.")
		fmt.Fprintln(writer, "Install the extension in at least one browser profile or run `open-browser-use setup`.")
		return nil
	}
	browserWidth := len("BROWSER")
	dirWidth := len("DIRECTORY")
	nameWidth := len("DISPLAY NAME")
	for _, profile := range profiles {
		browserLabel := profile.browserLabel()
		if w := len(browserLabel); w > browserWidth {
			browserWidth = w
		}
		if w := len(profile.Directory); w > dirWidth {
			dirWidth = w
		}
		name := profile.DisplayName
		if name == "" {
			name = "(unnamed)"
		}
		if w := len(name); w > nameWidth {
			nameWidth = w
		}
	}
	if showConnected {
		format := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-7s  %%s\n", browserWidth, dirWidth, nameWidth)
		fmt.Fprintf(writer, format, "BROWSER", "DIRECTORY", "DISPLAY NAME", "VERSION", "CONNECTED")
		for _, profile := range profiles {
			name := profile.DisplayName
			if name == "" {
				name = "(unnamed)"
			}
			marker := "-"
			if _, ok := connectedByTarget[profile.Target]; ok {
				marker = "yes"
			}
			fmt.Fprintf(writer, format, profile.browserLabel(), profile.Directory, name, profile.Version, marker)
		}
		for _, info := range connected {
			if info.Target != "" {
				continue
			}
			if info.InstanceID == "" {
				continue
			}
			fmt.Fprintf(writer, "(unresolved host: socket=%s instance=%s)\n", info.SocketPath, info.InstanceID)
		}
		return nil
	}
	format := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%s\n", browserWidth, dirWidth, nameWidth)
	fmt.Fprintf(writer, format, "BROWSER", "DIRECTORY", "DISPLAY NAME", "VERSION")
	for _, profile := range profiles {
		name := profile.DisplayName
		if name == "" {
			name = "(unnamed)"
		}
		fmt.Fprintf(writer, format, profile.browserLabel(), profile.Directory, name, profile.Version)
	}
	return nil
}

func (profile installedChromeProfile) browserLabel() string {
	if profile.BrowserName != "" {
		if profile.BrowserInstance != "" {
			return profile.BrowserName + "/" + profile.BrowserInstance
		}
		return profile.BrowserName
	}
	if profile.Browser != "" {
		if profile.BrowserInstance != "" {
			return profile.Browser + "/" + profile.BrowserInstance
		}
		return profile.Browser
	}
	return "(unknown)"
}

type actionRunner struct {
	options      socketOptions
	sessionID    string
	turnID       string
	currentTabID int
}

type actionRunOutput struct {
	Steps        []actionStepOutput `json:"steps"`
	CurrentTabID int                `json:"currentTabId,omitempty"`
}

type actionStepOutput struct {
	Line     int            `json:"line"`
	Action   string         `json:"action"`
	TabID    int            `json:"tabId,omitempty"`
	Response map[string]any `json:"response"`
}

func newActionRunner(options socketOptions) *actionRunner {
	sessionID := options.sessionID
	if sessionID == "" {
		sessionID = defaultCLISessionID
	}
	return &actionRunner{
		options:   options,
		sessionID: sessionID,
		turnID:    fmt.Sprintf("%s-%d", sessionID, time.Now().UnixNano()),
	}
}

func (runner *actionRunner) run(script string) (actionRunOutput, error) {
	lines, err := parseActionScript(script)
	if err != nil {
		return actionRunOutput{}, err
	}
	output := actionRunOutput{Steps: []actionStepOutput{}}
	for _, line := range lines {
		step, err := runner.runActionLine(line)
		if err != nil {
			return actionRunOutput{}, fmt.Errorf("line %d: %w", line.number, err)
		}
		output.Steps = append(output.Steps, step)
	}
	output.CurrentTabID = runner.currentTabID
	return output, nil
}

func (runner *actionRunner) runActionLine(line actionLine) (actionStepOutput, error) {
	action := line.fields[0]
	args := line.fields[1:]
	response, tabID, err := runner.runAction(action, args)
	if err != nil {
		return actionStepOutput{}, err
	}
	return actionStepOutput{
		Line:     line.number,
		Action:   action,
		TabID:    tabID,
		Response: response,
	}, nil
}

func (runner *actionRunner) runAction(action string, args []string) (map[string]any, int, error) {
	switch action {
	case "ping":
		return runner.invoke("ping", map[string]any{})
	case "info":
		return runner.invoke("getInfo", map[string]any{})
	case "tabs":
		return runner.invoke("getTabs", map[string]any{})
	case "user-tabs":
		return runner.invoke("getUserTabs", map[string]any{})
	case "turn-ended":
		return runner.invoke("turnEnded", map[string]any{})
	case "name-session":
		if len(args) == 0 {
			return nil, 0, errors.New("name-session requires a name")
		}
		return runner.invoke("nameSession", map[string]any{"name": strings.Join(args, " ")})
	case "open-tab":
		return runner.runOpenTabAction(args)
	case "claim-tab":
		tabID, err := firstIntArg(args, "--tab-id")
		if err != nil {
			return nil, 0, fmt.Errorf("claim-tab requires tab id: %w", err)
		}
		response, _, err := runner.invoke("claimUserTab", map[string]any{"tabId": tabID})
		if err == nil {
			runner.currentTabID = tabID
		}
		return response, tabID, err
	case "navigate":
		return runner.runNavigateAction(args)
	case "wait-load":
		return runner.runWaitLoadAction(args)
	case "page-info":
		return runner.runPageInfoAction(args)
	case "cdp":
		return runner.runCDPAction(args)
	case "history":
		return runner.runHistoryAction(args)
	case "move-mouse":
		return runner.runMoveMouseAction(args)
	case "wait-file-chooser":
		return runner.runWaitFileChooserAction(args)
	case "set-file-chooser-files":
		return runner.runSetFileChooserFilesAction(args)
	case "finalize-tabs":
		return runner.runFinalizeTabsAction(args)
	case "call":
		return runner.runCallAction(args)
	default:
		return nil, 0, fmt.Errorf("unsupported action %q", action)
	}
}

func (runner *actionRunner) runOpenTabAction(args []string) (map[string]any, int, error) {
	url := firstStringArg(args, "--url")
	response, _, err := runner.invoke("createTab", map[string]any{})
	if err != nil {
		return nil, 0, err
	}
	result, _ := response["result"].(map[string]any)
	tabID, ok := numberAsInt(result["id"])
	if !ok {
		return nil, 0, errors.New("createTab response did not include numeric tab id")
	}
	runner.currentTabID = tabID
	output := map[string]any{"tab": result}
	if url != "" {
		if err := runner.attach(tabID); err != nil {
			return nil, 0, err
		}
		navigate, _, err := runner.invoke("executeCdp", map[string]any{
			"target":        map[string]any{"tabId": tabID},
			"method":        "Page.navigate",
			"commandParams": map[string]any{"url": url},
		})
		if err != nil {
			return nil, 0, err
		}
		output["navigate"] = navigate["result"]
	}
	return map[string]any{"result": output}, tabID, nil
}

func (runner *actionRunner) runNavigateAction(args []string) (map[string]any, int, error) {
	tabID, err := tabIDArgOrCurrent(args, runner.currentTabID)
	if err != nil {
		return nil, 0, err
	}
	url := firstStringArg(args, "--url")
	if url == "" {
		return nil, 0, errors.New("navigate requires a URL")
	}
	if err := runner.attach(tabID); err != nil {
		return nil, 0, err
	}
	return runner.invoke("executeCdp", map[string]any{
		"target":        map[string]any{"tabId": tabID},
		"method":        "Page.navigate",
		"commandParams": map[string]any{"url": url},
	})
}

func (runner *actionRunner) runWaitLoadAction(args []string) (map[string]any, int, error) {
	tabID, err := tabIDArgOrCurrent(args, runner.currentTabID)
	if err != nil {
		return nil, 0, err
	}
	state := firstStringArg(args, "--state")
	if state == "" {
		state = "load"
	}
	if state != "load" && state != "domcontentloaded" {
		return nil, 0, fmt.Errorf("unsupported load state %q", state)
	}
	if err := runner.attach(tabID); err != nil {
		return nil, 0, err
	}
	if _, _, err := runner.invoke("executeCdp", map[string]any{
		"target":        map[string]any{"tabId": tabID},
		"method":        "Page.enable",
		"commandParams": map[string]any{},
	}); err != nil {
		return nil, 0, err
	}
	deadline := time.Now().Add(runner.options.timeout)
	for {
		response, _, err := runner.invoke("executeCdp", map[string]any{
			"target": map[string]any{"tabId": tabID},
			"method": "Runtime.evaluate",
			"commandParams": map[string]any{
				"expression":    "document.readyState",
				"returnByValue": true,
			},
		})
		if err != nil {
			return nil, 0, err
		}
		readyState := runtimeEvaluateString(response)
		if readyState == "complete" || (state == "domcontentloaded" && readyState == "interactive") {
			return map[string]any{"result": map[string]any{"readyState": readyState}}, tabID, nil
		}
		if time.Now().After(deadline) {
			return nil, 0, fmt.Errorf("timed out waiting for %s in tab %d", state, tabID)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (runner *actionRunner) runPageInfoAction(args []string) (map[string]any, int, error) {
	tabID, err := tabIDArgOrCurrent(args, runner.currentTabID)
	if err != nil {
		return nil, 0, err
	}
	if err := runner.attach(tabID); err != nil {
		return nil, 0, err
	}
	response, _, err := runner.invoke("executeCdp", map[string]any{
		"target": map[string]any{"tabId": tabID},
		"method": "Runtime.evaluate",
		"commandParams": map[string]any{
			"expression":    `({ title: document.title ?? "", url: location.href, readyState: document.readyState, text: document.body?.innerText ?? "" })`,
			"returnByValue": true,
		},
	})
	return response, tabID, err
}

func (runner *actionRunner) runCDPAction(args []string) (map[string]any, int, error) {
	method := stringFlagOrPositional(args, "--method", 0)
	if method == "" {
		return nil, 0, errors.New("cdp requires a method")
	}
	paramsJSON := stringFlagOrPositional(args, "--params", 1)
	if paramsJSON == "" {
		paramsJSON = "{}"
	}
	var commandParams map[string]any
	if err := json.Unmarshal([]byte(paramsJSON), &commandParams); err != nil {
		return nil, 0, fmt.Errorf("cdp params must be a JSON object: %w", err)
	}
	tabID, err := tabIDArgOrCurrent(args, runner.currentTabID)
	if err != nil {
		return nil, 0, err
	}
	if err := runner.attach(tabID); err != nil {
		return nil, 0, err
	}
	return runner.invoke("executeCdp", map[string]any{
		"target":        map[string]any{"tabId": tabID},
		"method":        method,
		"commandParams": commandParams,
	})
}

func (runner *actionRunner) runHistoryAction(args []string) (map[string]any, int, error) {
	params := map[string]any{"query": firstStringArg(args, "--query"), "limit": 100}
	if limit, ok := intFlag(args, "--limit"); ok {
		params["limit"] = limit
	}
	if from := stringFlag(args, "--from"); from != "" {
		params["from"] = from
	}
	if to := stringFlag(args, "--to"); to != "" {
		params["to"] = to
	}
	return runner.invoke("getUserHistory", params)
}

func (runner *actionRunner) runMoveMouseAction(args []string) (map[string]any, int, error) {
	tabID, err := tabIDArgOrCurrent(args, runner.currentTabID)
	if err != nil {
		return nil, 0, err
	}
	if len(positionalArgs(args)) < 2 {
		return nil, 0, errors.New("move-mouse requires x and y")
	}
	x, err := strconv.ParseFloat(positionalArgs(args)[0], 64)
	if err != nil {
		return nil, 0, fmt.Errorf("move-mouse x must be numeric: %w", err)
	}
	y, err := strconv.ParseFloat(positionalArgs(args)[1], 64)
	if err != nil {
		return nil, 0, fmt.Errorf("move-mouse y must be numeric: %w", err)
	}
	return runner.invoke("moveMouse", map[string]any{
		"tabId":          tabID,
		"x":              x,
		"y":              y,
		"waitForArrival": true,
	})
}

func (runner *actionRunner) runWaitFileChooserAction(args []string) (map[string]any, int, error) {
	tabID, err := tabIDArgOrCurrent(args, runner.currentTabID)
	if err != nil {
		return nil, 0, err
	}
	return runner.invoke("waitForFileChooser", map[string]any{
		"tabId":     tabID,
		"timeoutMs": int(runner.options.timeout.Milliseconds()),
	})
}

func (runner *actionRunner) runSetFileChooserFilesAction(args []string) (map[string]any, int, error) {
	fileChooserID := stringFlagOrPositional(args, "--file-chooser-id", 0)
	if fileChooserID == "" {
		return nil, 0, errors.New("set-file-chooser-files requires file chooser id")
	}
	files := positionalArgs(args)
	if stringFlag(args, "--file-chooser-id") == "" && len(files) > 0 {
		files = files[1:]
	}
	if len(files) == 0 {
		return nil, 0, errors.New("set-file-chooser-files requires at least one file")
	}
	return runner.invoke("setFileChooserFiles", map[string]any{
		"fileChooserId": fileChooserID,
		"files":         files,
	})
}

func (runner *actionRunner) runFinalizeTabsAction(args []string) (map[string]any, int, error) {
	keepJSON := firstStringArg(args, "--keep")
	if keepJSON == "" {
		keepJSON = "[]"
	}
	var keep []any
	if err := json.Unmarshal([]byte(keepJSON), &keep); err != nil {
		return nil, 0, fmt.Errorf("finalize-tabs keep must be a JSON array: %w", err)
	}
	return runner.invoke("finalizeTabs", map[string]any{"keep": keep})
}

func (runner *actionRunner) runCallAction(args []string) (map[string]any, int, error) {
	method := stringFlagOrPositional(args, "--method", 0)
	if method == "" {
		return nil, 0, errors.New("call requires a method")
	}
	paramsJSON := stringFlagOrPositional(args, "--params", 1)
	if paramsJSON == "" {
		paramsJSON = "{}"
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		return nil, 0, fmt.Errorf("call params must be a JSON object: %w", err)
	}
	return runner.invoke(method, params)
}

func (runner *actionRunner) attach(tabID int) error {
	_, _, err := runner.invoke("attach", map[string]any{"tabId": tabID})
	return err
}

func (runner *actionRunner) invoke(method string, params map[string]any) (map[string]any, int, error) {
	if params == nil {
		params = map[string]any{}
	}
	params["session_id"] = runner.sessionID
	params["turn_id"] = runner.turnID
	response, err := invokeWithProfile(runner.options.socketPath, runner.options.socketDir, runner.options.browser, runner.options.profile, method, params, runner.options.timeout)
	return response, runner.currentTabID, err
}

type actionLine struct {
	number int
	fields []string
}

func parseActionScript(script string) ([]actionLine, error) {
	var lines []actionLine
	for index, rawLine := range strings.Split(script, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields, err := splitActionFields(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", index+1, err)
		}
		if len(fields) == 0 {
			continue
		}
		lines = append(lines, actionLine{number: index + 1, fields: fields})
	}
	return lines, nil
}

func splitActionFields(line string) ([]string, error) {
	var fields []string
	var builder strings.Builder
	var quote rune
	escaped := false
	emit := func() {
		if builder.Len() > 0 {
			fields = append(fields, builder.String())
			builder.Reset()
		}
	}
	for _, current := range line {
		if escaped {
			builder.WriteRune(current)
			escaped = false
			continue
		}
		if current == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if current == quote {
				quote = 0
			} else {
				builder.WriteRune(current)
			}
			continue
		}
		if current == '"' || current == '\'' {
			quote = current
			continue
		}
		if current == '#' && builder.Len() == 0 {
			break
		}
		if current == ' ' || current == '\t' {
			emit()
			continue
		}
		builder.WriteRune(current)
	}
	if escaped {
		return nil, errors.New("unterminated escape")
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	emit()
	return fields, nil
}

func firstStringArg(args []string, flag string) string {
	if value := stringFlag(args, flag); value != "" {
		return value
	}
	for _, arg := range positionalArgs(args) {
		return arg
	}
	return ""
}

func stringFlagOrPositional(args []string, flag string, positionalIndex int) string {
	if value := stringFlag(args, flag); value != "" {
		return value
	}
	positionals := positionalArgs(args)
	if positionalIndex >= 0 && positionalIndex < len(positionals) {
		return positionals[positionalIndex]
	}
	return ""
}

func firstIntArg(args []string, flag string) (int, error) {
	if value, ok := intFlag(args, flag); ok {
		return value, nil
	}
	for _, arg := range positionalArgs(args) {
		value, err := strconv.Atoi(arg)
		if err != nil {
			return 0, err
		}
		return value, nil
	}
	return 0, errors.New("missing integer argument")
}

func tabIDArgOrCurrent(args []string, currentTabID int) (int, error) {
	if value, ok := intFlag(args, "--tab-id"); ok {
		return value, nil
	}
	if currentTabID > 0 {
		return currentTabID, nil
	}
	return 0, errors.New("action requires --tab-id or a previous open-tab/claim-tab action")
}

func stringFlag(args []string, name string) string {
	for index, arg := range args {
		if arg == name && index+1 < len(args) {
			return args[index+1]
		}
		prefix := name + "="
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
	}
	return ""
}

func intFlag(args []string, name string) (int, bool) {
	value := stringFlag(args, name)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func positionalArgs(args []string) []string {
	var out []string
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if strings.HasPrefix(arg, "--") {
			if !strings.Contains(arg, "=") {
				index++
			}
			continue
		}
		out = append(out, arg)
	}
	return out
}

func runtimeEvaluateString(response map[string]any) string {
	result, _ := response["result"].(map[string]any)
	cdpResult, _ := result["result"].(map[string]any)
	value, _ := cdpResult["value"].(string)
	return value
}

func invokeAndWrite(options socketOptions, method string, params map[string]any) error {
	response, err := invokeWithOptions(options, method, params)
	if err != nil {
		return err
	}
	return writeJSON(response)
}

func invokeWithOptions(options socketOptions, method string, params map[string]any) (map[string]any, error) {
	applySessionDefaults(params, options.sessionID)
	return invokeWithProfile(options.socketPath, options.socketDir, options.browser, options.profile, method, params, options.timeout)
}

func invoke(socketPath string, socketDir string, method string, params map[string]any, timeout time.Duration) (map[string]any, error) {
	return invokeWithProfile(socketPath, socketDir, "", "", method, params, timeout)
}

func invokeWithProfile(socketPath string, socketDir string, browser string, profile string, method string, params map[string]any, timeout time.Duration) (map[string]any, error) {
	conn, err := dialBrowserSocketForProfile(socketPath, socketDir, browser, profile, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	applySessionDefaults(params, defaultCLISessionID)
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      "cli-1",
		"method":  method,
		"params":  params,
	}
	if err := wire.WriteJSON(conn, request); err != nil {
		return nil, err
	}
	var response map[string]any
	if err := wire.ReadJSON(conn, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func applySessionDefaults(params map[string]any, sessionID string) {
	if sessionID == "" {
		sessionID = defaultCLISessionID
	}
	if _, ok := params["session_id"]; !ok {
		params["session_id"] = sessionID
	}
	if _, ok := params["turn_id"]; !ok {
		params["turn_id"] = fmt.Sprintf("%s-%d", sessionID, time.Now().UnixNano())
	}
}

func dialBrowserSocketForProfile(socketPath string, socketDir string, browser string, profile string, timeout time.Duration) (net.Conn, error) {
	if strings.TrimSpace(browser) == "" && strings.TrimSpace(profile) == "" {
		return dialBrowserSocket(socketPath, socketDir, timeout)
	}
	// When the caller pinned an explicit --socket, honour it but still verify it
	// is the right browser/profile. This protects the user from accidentally
	// talking to the wrong browser.
	if socketPath != "" {
		conn, err := net.DialTimeout("unix", socketPath, timeout)
		if err != nil {
			return nil, err
		}
		match, info, verifyErr := verifySocketMatchesProfile(conn, browser, profile, timeout)
		if verifyErr != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("could not verify profile of socket %s: %w", socketPath, verifyErr)
		}
		if !match {
			_ = conn.Close()
			return nil, fmt.Errorf("socket %s belongs to %s; adjust --browser/--profile or omit --socket to auto-route", socketPath, info.label())
		}
		return conn, nil
	}
	conn, _, _, err := pickSocketForProfile(socketDir, browser, profile, timeout)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func dialBrowserSocket(socketPath string, socketDir string, timeout time.Duration) (net.Conn, error) {
	if socketPath != "" {
		return net.DialTimeout("unix", socketPath, timeout)
	}

	record, err := host.ReadActiveSocketRecord(socketDir)
	if err == nil {
		conn, dialErr := net.DialTimeout("unix", record.SocketPath, timeout)
		if dialErr == nil {
			return conn, nil
		}
		_ = host.RemoveActiveSocketRecord(socketDir, record.SocketPath)
		removeSocketPathIfInDir(socketDir, record.SocketPath)
		conn, scanErr := scanSocketDir(socketDir, record.SocketPath, timeout)
		if scanErr == nil {
			return conn, nil
		}
		return nil, fmt.Errorf("active socket registry points to unavailable socket %q; removed stale registry entry; no connectable socket found by scanning: %w", record.SocketPath, dialErr)
	}

	conn, scanErr := scanSocketDir(socketDir, "", timeout)
	if scanErr == nil {
		return conn, nil
	}
	return nil, fmt.Errorf("socket not provided and active socket registry is unavailable; no connectable socket found by scanning: %w", err)
}

type socketCandidate struct {
	path    string
	modTime time.Time
}

type connectedProfileInfo struct {
	SocketPath      string `json:"socketPath"`
	InstanceID      string `json:"instanceId,omitempty"`
	ExtensionID     string `json:"extensionId,omitempty"`
	Browser         string `json:"browser,omitempty"`
	BrowserName     string `json:"browserName,omitempty"`
	BrowserInstance string `json:"browserInstance,omitempty"`
	Directory       string `json:"directory,omitempty"`
	DisplayName     string `json:"displayName,omitempty"`
	Target          string `json:"target,omitempty"`
}

func (info connectedProfileInfo) label() string {
	prefix := info.BrowserName
	if prefix == "" {
		prefix = info.Browser
	}
	if info.BrowserInstance != "" {
		prefix = fmt.Sprintf("%s/%s", prefix, info.BrowserInstance)
	}
	if info.DisplayName != "" && info.Directory != "" {
		return strings.TrimSpace(fmt.Sprintf("%s %s (%s)", prefix, info.Directory, info.DisplayName))
	}
	if info.Directory != "" {
		return strings.TrimSpace(fmt.Sprintf("%s %s", prefix, info.Directory))
	}
	if info.DisplayName != "" {
		return strings.TrimSpace(fmt.Sprintf("%s %s", prefix, info.DisplayName))
	}
	if prefix != "" {
		return prefix
	}
	if info.InstanceID != "" {
		return "instance " + info.InstanceID
	}
	return "unknown"
}

func getInfoOverConn(conn net.Conn, timeout time.Duration) (map[string]any, error) {
	deadline := time.Now().Add(timeout)
	_ = conn.SetDeadline(deadline)
	defer func() { _ = conn.SetDeadline(time.Time{}) }()
	params := map[string]any{}
	applySessionDefaults(params, defaultCLISessionID)
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      fmt.Sprintf("cli-profile-%d", time.Now().UnixNano()),
		"method":  "getInfo",
		"params":  params,
	}
	wantID, _ := req["id"].(string)
	if err := wire.WriteJSON(conn, req); err != nil {
		return nil, err
	}
	for {
		var resp map[string]any
		if err := wire.ReadJSON(conn, &resp); err != nil {
			return nil, err
		}
		if id, ok := resp["id"].(string); ok && id == wantID {
			return resp, nil
		}
		// skip notifications / stale responses while waiting for our id
	}
}

func connectedProfileFromInfo(socketPath string, payload map[string]any) connectedProfileInfo {
	info := connectedProfileInfo{SocketPath: socketPath}
	result, _ := payload["result"].(map[string]any)
	metadata, _ := result["metadata"].(map[string]any)
	if extID, ok := metadata["extensionId"].(string); ok {
		info.ExtensionID = extID
	}
	if instanceID, ok := metadata["extensionInstanceId"].(string); ok {
		info.InstanceID = instanceID
	}
	if browserID, browserName, browserInstance, dir, name, ok := resolveProfileForInstanceID(info.ExtensionID, info.InstanceID); ok {
		info.Browser = browserID
		info.BrowserName = browserName
		info.BrowserInstance = browserInstance
		info.Directory = dir
		info.DisplayName = name
		info.Target = browserProfileTarget(browserID, browserInstance, dir)
	}
	return info
}

func browserSelectorMatches(selector string, info connectedProfileInfo) bool {
	target := strings.TrimSpace(selector)
	if target == "" {
		return true
	}
	if info.Browser != "" && strings.EqualFold(target, info.Browser) {
		return true
	}
	if info.BrowserName != "" && strings.EqualFold(target, info.BrowserName) {
		return true
	}
	if info.BrowserInstance != "" && strings.EqualFold(target, info.BrowserInstance) {
		return true
	}
	if info.Target != "" && strings.EqualFold(target, info.Target) {
		return true
	}
	return false
}

func profileSelectorMatches(selector string, info connectedProfileInfo) bool {
	target := strings.TrimSpace(selector)
	if target == "" {
		return true
	}
	if info.Directory != "" && strings.EqualFold(target, info.Directory) {
		return true
	}
	if info.DisplayName != "" && strings.EqualFold(target, info.DisplayName) {
		return true
	}
	if info.InstanceID != "" && strings.EqualFold(target, info.InstanceID) {
		return true
	}
	return false
}

func verifySocketMatchesProfile(conn net.Conn, browserSelector string, profileSelector string, timeout time.Duration) (bool, connectedProfileInfo, error) {
	resp, err := getInfoOverConn(conn, timeout)
	if err != nil {
		return false, connectedProfileInfo{}, err
	}
	info := connectedProfileFromInfo("", resp)
	return browserSelectorMatches(browserSelector, info) && profileSelectorMatches(profileSelector, info), info, nil
}

func pickSocketForProfile(socketDir string, browserSelector string, profileSelector string, timeout time.Duration) (net.Conn, connectedProfileInfo, []connectedProfileInfo, error) {
	candidates, err := listSocketCandidates(socketDir)
	if err != nil {
		return nil, connectedProfileInfo{}, nil, err
	}
	if len(candidates) == 0 {
		return nil, connectedProfileInfo{}, nil, fmt.Errorf("no Open Browser Use socket found; open Chrome with the desired profile (`open-browser-use profiles` lists installed profiles)")
	}
	probeTimeout := timeout
	if probeTimeout > 800*time.Millisecond {
		probeTimeout = 800 * time.Millisecond
	}
	var seen []connectedProfileInfo
	for _, candidate := range candidates {
		conn, err := net.DialTimeout("unix", candidate.path, probeTimeout)
		if err != nil {
			removeSocketPathIfInDir(socketDir, candidate.path)
			continue
		}
		resp, err := getInfoOverConn(conn, probeTimeout)
		if err != nil {
			_ = conn.Close()
			continue
		}
		info := connectedProfileFromInfo(candidate.path, resp)
		seen = append(seen, info)
		if browserSelectorMatches(browserSelector, info) && profileSelectorMatches(profileSelector, info) {
			repairActiveSocketRecord(socketDir, candidate)
			return conn, info, seen, nil
		}
		_ = conn.Close()
	}
	return nil, connectedProfileInfo{}, seen, fmt.Errorf("no running Open Browser Use host matched --browser %q --profile %q; %s", browserSelector, profileSelector, profileMatchHint(seen))
}

func profileMatchHint(seen []connectedProfileInfo) string {
	if len(seen) == 0 {
		return "no Chrome host was reachable; open Chrome on the desired profile and retry"
	}
	labels := make([]string, 0, len(seen))
	for _, info := range seen {
		labels = append(labels, info.label())
	}
	return "currently connected: " + strings.Join(labels, ", ")
}

func listSocketCandidates(socketDir string) ([]socketCandidate, error) {
	dir := socketDir
	if dir == "" {
		dir = host.DefaultSocketDir
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var candidates []socketCandidate
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sock") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, socketCandidate{
			path:    filepath.Join(dir, entry.Name()),
			modTime: info.ModTime(),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].path > candidates[j].path
		}
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	return candidates, nil
}

func scanSocketDir(socketDir string, skipPath string, timeout time.Duration) (net.Conn, error) {
	dir := socketDir
	if dir == "" {
		dir = host.DefaultSocketDir
	}
	candidates, err := listSocketCandidates(socketDir)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no socket files in %q", dir)
	}

	probeTimeout := timeout
	if probeTimeout > 500*time.Millisecond {
		probeTimeout = 500 * time.Millisecond
	}
	var lastErr error
	for _, candidate := range candidates {
		if candidate.path == skipPath {
			continue
		}
		conn, err := net.DialTimeout("unix", candidate.path, probeTimeout)
		if err == nil {
			repairActiveSocketRecord(dir, candidate)
			cleanupStaleSocketCandidates(dir, candidates, candidate.path, probeTimeout)
			return conn, nil
		}
		removeSocketPathIfInDir(dir, candidate.path)
		lastErr = err
	}
	if lastErr == nil {
		return nil, fmt.Errorf("no socket files in %q after filtering", dir)
	}
	return nil, lastErr
}

func cleanupStaleSocketCandidates(socketDir string, candidates []socketCandidate, keepPath string, timeout time.Duration) {
	cleanupTimeout := timeout
	if cleanupTimeout > 100*time.Millisecond {
		cleanupTimeout = 100 * time.Millisecond
	}
	for _, candidate := range candidates {
		if candidate.path == keepPath {
			continue
		}
		conn, err := net.DialTimeout("unix", candidate.path, cleanupTimeout)
		if err == nil {
			_ = conn.Close()
			continue
		}
		removeSocketPathIfInDir(socketDir, candidate.path)
	}
}

func repairActiveSocketRecord(socketDir string, candidate socketCandidate) {
	record := host.ActiveSocketRecord{
		SocketPath: candidate.path,
		PID:        0,
		StartedAt:  candidate.modTime.UTC(),
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return
	}
	path := host.ActiveSocketRecordPath(socketDir)
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		return
	}
	_ = os.Chmod(path, 0o600)
}

func removeSocketPathIfInDir(socketDir string, socketPath string) {
	dir := socketDir
	if dir == "" {
		dir = host.DefaultSocketDir
	}
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	pathAbs, err := filepath.Abs(socketPath)
	if err != nil {
		return
	}
	rel, err := filepath.Rel(dirAbs, pathAbs)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return
	}
	if filepath.Dir(rel) != "." || !strings.HasSuffix(rel, ".sock") {
		return
	}
	_ = os.Remove(pathAbs)
}

func writeJSON(value any) error {
	return writeJSONTo(os.Stdout, value)
}

func writeJSONTo(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func nativeManifest(extensionID string, hostPath string) (map[string]any, error) {
	allowedExtensionID := strings.TrimSpace(extensionID)
	if allowedExtensionID == "" {
		allowedExtensionID = defaultChromeExtensionID
	}
	path := hostPath
	if path == "" {
		var err error
		path, err = stableNativeHostPath()
		if err != nil {
			return nil, err
		}
	} else {
		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		path = absolutePath
	}
	return map[string]any{
		"name":        host.NativeHostName,
		"description": "Open Browser Use Chrome native messaging host",
		"type":        "stdio",
		"path":        path,
		"allowed_origins": []string{
			fmt.Sprintf("chrome-extension://%s/", allowedExtensionID),
		},
	}, nil
}

func resolveNativeHostTarget(binaryPath string) (string, error) {
	path := binaryPath
	if path == "" {
		executable, err := os.Executable()
		if err != nil {
			return "", err
		}
		path = executable
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("native host binary target is a directory: %s", absolutePath)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("native host binary target is not executable: %s", absolutePath)
	}
	return absolutePath, nil
}

func stableNativeHostPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library/Application Support/OpenBrowserUse/native-host/open-browser-use"), nil
	case "linux":
		return filepath.Join(home, ".local/share/open-browser-use/native-host/open-browser-use"), nil
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if strings.TrimSpace(localAppData) == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "OpenBrowserUse", "native-host", "open-browser-use.exe"), nil
	default:
		return "", fmt.Errorf("stable native host link path is not implemented for %s", runtime.GOOS)
	}
}

func defaultChromeExternalExtensionPath(extensionID string) (string, error) {
	return defaultChromeExternalExtensionPathForBrowser(extensionID, "")
}

func defaultChromeExternalExtensionPathForBrowser(extensionID string, browserSelector string) (string, error) {
	filename := strings.TrimSpace(extensionID) + ".json"
	if filename == ".json" {
		return "", errors.New("Chrome extension id is empty")
	}
	switch runtime.GOOS {
	case "darwin":
		root, err := browserRootForInstallSelector(browserSelector)
		if err != nil {
			return "", err
		}
		if root.BrowserID == "bitbrowser" {
			return "", errors.New("BitBrowser external extension setup is not supported; install the extension in BitBrowser and use install-manifest --browser bitbrowser")
		}
		return filepath.Join(root.Root, "External Extensions", filename), nil
	case "linux":
		return filepath.Join("/opt/google/chrome/extensions", filename), nil
	case "windows":
		if strings.TrimSpace(browserSelector) != "" && !browserSelectorMatches(browserSelector, connectedProfileInfo{Browser: "chrome", BrowserName: "Google Chrome"}) {
			return "", fmt.Errorf("Chrome external extension setup is not implemented for browser selector %q on %s", browserSelector, runtime.GOOS)
		}
		return `HKCU\Software\Google\Chrome\Extensions\` + strings.TrimSpace(extensionID), nil
	default:
		return "", fmt.Errorf("Chrome external extension setup is not implemented for %s", runtime.GOOS)
	}
}

func installStableNativeHostLink(targetPath string, linkPath string) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o700); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		if samePath(targetPath, linkPath) {
			return nil
		}
		return copyFile(targetPath, linkPath, 0o700)
	}
	if err := os.Remove(linkPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Symlink(targetPath, linkPath)
}

func samePath(left string, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(leftAbs), filepath.Clean(rightAbs))
}

func copyFile(sourcePath string, targetPath string, mode os.FileMode) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func defaultNativeHostManifestPath() (string, error) {
	return defaultNativeHostManifestPathForBrowser("")
}

func defaultNativeHostManifestPathForBrowser(browserSelector string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	filename := host.NativeHostName + ".json"
	switch runtime.GOOS {
	case "darwin":
		root, err := browserRootForInstallSelector(browserSelector)
		if err != nil {
			return "", err
		}
		return filepath.Join(root.Root, "NativeMessagingHosts", filename), nil
	case "linux":
		return filepath.Join(home, ".config/google-chrome/NativeMessagingHosts", filename), nil
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if strings.TrimSpace(localAppData) == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "OpenBrowserUse", "NativeMessagingHosts", filename), nil
	default:
		return "", fmt.Errorf("default manifest install path is not implemented for %s; pass --output", runtime.GOOS)
	}
}

func browserRootForInstallSelector(selector string) (browserProfileRoot, error) {
	target := strings.TrimSpace(selector)
	if target == "" {
		target = "chrome"
	}
	roots, err := supportedBrowserProfileRoots()
	if err != nil {
		return browserProfileRoot{}, err
	}
	var matches []browserProfileRoot
	for _, root := range roots {
		info := connectedProfileInfo{
			Browser:         root.BrowserID,
			BrowserName:     root.BrowserName,
			BrowserInstance: root.BrowserInstance,
			Target:          browserProfileTarget(root.BrowserID, root.BrowserInstance, ""),
		}
		if browserSelectorMatches(target, info) {
			matches = append(matches, root)
		}
	}
	if len(matches) == 0 {
		return browserProfileRoot{}, fmt.Errorf("unknown browser selector %q", selector)
	}
	if len(matches) > 1 {
		labels := make([]string, 0, len(matches))
		for _, match := range matches {
			label := match.BrowserID
			if match.BrowserInstance != "" {
				label += ":" + match.BrowserInstance
			}
			labels = append(labels, label)
		}
		return browserProfileRoot{}, fmt.Errorf("browser selector %q is ambiguous; use one of: %s", selector, strings.Join(labels, ", "))
	}
	return matches[0], nil
}

func downloadLatestReleaseZIP() (string, error) {
	assetName := fmt.Sprintf("open-browser-use-chrome-extension-%s.zip", version)
	url := fmt.Sprintf(
		"https://github.com/iFurySt/open-browser-use/releases/download/v%s/%s",
		version,
		assetName,
	)
	return downloadReleaseAsset(assetName, url)
}

func downloadReleaseAsset(name string, url string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	targetDir := filepath.Join(cacheDir, "open-browser-use", "extensions")
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return "", err
	}
	targetPath := filepath.Join(targetDir, filepath.Base(name))
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "open-browser-use/"+version)
	client := http.Client{Timeout: 2 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("extension ZIP download failed: %s", response.Status)
	}
	tempPath := targetPath + ".tmp"
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(file, response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tempPath)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tempPath)
		return "", closeErr
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}
	return targetPath, nil
}

func resolveExistingExtensionZIP(path string) (string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("extension ZIP path is a directory: %s", absolutePath)
	}
	return absolutePath, nil
}

func installUnpackedExtension(zipPath string) (string, string, error) {
	targetDir, err := defaultUnpackedExtensionDir()
	if err != nil {
		return "", "", err
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return "", "", err
	}
	if err := unzipExtension(zipPath, targetDir); err != nil {
		return "", "", err
	}
	extensionID, err := pinUnpackedExtensionKey(filepath.Join(targetDir, "manifest.json"))
	if err != nil {
		return "", "", err
	}
	return targetDir, extensionID, nil
}

func writeBetaInstallZIP(unpackedPath string, sourceZIPPath string) (string, error) {
	targetPath := sourceZIPPath
	tempPath := targetPath + ".tmp"
	if err := os.Remove(tempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := packageExtensionDirectory(unpackedPath, tempPath); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}
	return targetPath, nil
}

func packageExtensionDirectory(sourceDir string, targetZIPPath string) error {
	target, err := os.Create(targetZIPPath)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(target)
	walkErr := filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relativePath)
		header.Method = zip.Deflate
		destination, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(destination, source)
		closeErr := source.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	closeWriterErr := writer.Close()
	closeTargetErr := target.Close()
	if walkErr != nil {
		return walkErr
	}
	if closeWriterErr != nil {
		return closeWriterErr
	}
	return closeTargetErr
}

func defaultUnpackedExtensionDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library/Application Support/OpenBrowserUse/chrome-extension/release"), nil
	case "linux":
		return filepath.Join(home, ".local/share/open-browser-use/chrome-extension/release"), nil
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if strings.TrimSpace(localAppData) == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "OpenBrowserUse", "chrome-extension", "release"), nil
	default:
		return "", fmt.Errorf("release extension setup is not implemented for %s", runtime.GOOS)
	}
}

func unzipExtension(zipPath string, targetDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()
	cleanTargetDir, err := filepath.Abs(targetDir)
	if err != nil {
		return err
	}
	for _, file := range reader.File {
		targetPath := filepath.Join(cleanTargetDir, file.Name)
		if targetPath != cleanTargetDir && !strings.HasPrefix(targetPath, cleanTargetDir+string(os.PathSeparator)) {
			return fmt.Errorf("extension ZIP contains unsafe path: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
			return err
		}
		source, err := file.Open()
		if err != nil {
			return err
		}
		destination, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			_ = source.Close()
			return err
		}
		_, copyErr := io.Copy(destination, source)
		closeDestinationErr := destination.Close()
		closeSourceErr := source.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeDestinationErr != nil {
			return closeDestinationErr
		}
		if closeSourceErr != nil {
			return closeSourceErr
		}
	}
	return nil
}

func pinUnpackedExtensionKey(manifestPath string) (string, error) {
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", err
	}
	var manifest map[string]any
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return "", err
	}
	key, _ := manifest["key"].(string)
	if strings.TrimSpace(key) == "" {
		key = betaExtensionPublicKey
		manifest["key"] = key
		updated, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(manifestPath, append(updated, '\n'), 0o600); err != nil {
			return "", err
		}
	}
	return extensionIDFromPublicKey(key)
}

func extensionIDFromPublicKey(key string) (string, error) {
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(key))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(der)
	return extensionIDFromCRXID(sum[:16]), nil
}

func extensionIDFromCRX(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	prefix := make([]byte, 12)
	if _, err := io.ReadFull(file, prefix); err != nil {
		return "", err
	}
	if string(prefix[0:4]) != "Cr24" {
		return "", fmt.Errorf("CRX %s does not start with Cr24", path)
	}
	version := binary.LittleEndian.Uint32(prefix[4:8])
	if version != 3 {
		return "", fmt.Errorf("CRX %s uses unsupported version %d", path, version)
	}
	headerLength := binary.LittleEndian.Uint32(prefix[8:12])
	header := make([]byte, headerLength)
	if _, err := io.ReadFull(file, header); err != nil {
		return "", err
	}
	signedHeaderData, err := protobufBytesField(header, 10000)
	if err != nil {
		return "", err
	}
	crxID, err := protobufBytesField(signedHeaderData, 1)
	if err != nil {
		return "", err
	}
	if len(crxID) != 16 {
		return "", fmt.Errorf("CRX %s has invalid id length %d", path, len(crxID))
	}
	return extensionIDFromCRXID(crxID), nil
}

func protobufBytesField(payload []byte, wantedField uint64) ([]byte, error) {
	for offset := 0; offset < len(payload); {
		key, n, err := readVarint(payload[offset:])
		if err != nil {
			return nil, err
		}
		offset += n
		fieldNumber := key >> 3
		wireType := key & 0x7
		switch wireType {
		case 0:
			_, n, err := readVarint(payload[offset:])
			if err != nil {
				return nil, err
			}
			offset += n
		case 1:
			offset += 8
		case 2:
			length, n, err := readVarint(payload[offset:])
			if err != nil {
				return nil, err
			}
			offset += n
			if length > uint64(len(payload)-offset) {
				return nil, errors.New("protobuf bytes field exceeds payload")
			}
			end := offset + int(length)
			if fieldNumber == wantedField {
				return payload[offset:end], nil
			}
			offset = end
		case 5:
			offset += 4
		default:
			return nil, fmt.Errorf("unsupported protobuf wire type %d", wireType)
		}
		if offset > len(payload) {
			return nil, errors.New("protobuf field exceeds payload")
		}
	}
	return nil, fmt.Errorf("protobuf bytes field %d not found", wantedField)
}

func readVarint(payload []byte) (uint64, int, error) {
	var value uint64
	for i, b := range payload {
		if i == 10 {
			return 0, 0, errors.New("protobuf varint is too long")
		}
		value |= uint64(b&0x7f) << (7 * i)
		if b < 0x80 {
			return value, i + 1, nil
		}
	}
	return 0, 0, io.ErrUnexpectedEOF
}

func extensionIDFromCRXID(crxID []byte) string {
	const alphabet = "abcdefghijklmnop"
	var builder strings.Builder
	builder.Grow(len(crxID) * 2)
	for _, b := range crxID {
		builder.WriteByte(alphabet[(b>>4)&0x0f])
		builder.WriteByte(alphabet[b&0x0f])
	}
	return builder.String()
}

func openFile(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-a", "Google Chrome", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		return fmt.Errorf("opening files is not implemented for %s", runtime.GOOS)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func revealFile(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	case "linux":
		cmd = exec.Command("xdg-open", filepath.Dir(path))
	case "windows":
		cmd = exec.Command("explorer", "/select,", path)
	default:
		return fmt.Errorf("revealing files is not implemented for %s", runtime.GOOS)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func openChromeExtensionsPage() error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-a", "Google Chrome", "chrome://extensions/")
	case "linux":
		cmd = exec.Command("xdg-open", "chrome://extensions/")
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", "chrome://extensions/")
	default:
		return fmt.Errorf("opening Chrome extensions page is not implemented for %s", runtime.GOOS)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func openChromeWebStorePage() error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-a", "Google Chrome", chromeWebStoreExtensionURL)
	case "linux":
		cmd = exec.Command("xdg-open", chromeWebStoreExtensionURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", chromeWebStoreExtensionURL)
	default:
		return fmt.Errorf("opening Chrome Web Store is not implemented for %s", runtime.GOOS)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func regAddDefaultString(key string, value string) error {
	return regAddString(key, "", value)
}

func regAddString(key string, name string, value string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("registry setup is not implemented for %s", runtime.GOOS)
	}
	args := []string{"ADD", key}
	if strings.TrimSpace(name) == "" {
		args = append(args, "/ve")
	} else {
		args = append(args, "/v", name)
	}
	args = append(args, "/t", "REG_SZ", "/d", value, "/f")
	output, err := exec.Command("reg.exe", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func numberAsInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case float64:
		if typed == float64(int(typed)) {
			return int(typed), true
		}
	}
	return 0, false
}
