package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ifuryst/open-browser-use/internal/host"
	"github.com/ifuryst/open-browser-use/internal/wire"
)

func TestNativeHostNameIsChromeCompatible(t *testing.T) {
	valid := regexp.MustCompile(`^[a-z0-9_]+(\.[a-z0-9_]+)*$`)
	if !valid.MatchString(host.NativeHostName) {
		t.Fatalf("native host name is not Chrome-compatible: %q", host.NativeHostName)
	}
}

func TestNativeMessagingLaunchArg(t *testing.T) {
	if !isNativeMessagingLaunch([]string{"--parent-window=123", "chrome-extension://nfjjgckfgejeofdcmaepbapclmldcflf/"}) {
		t.Fatal("expected Chrome extension origin to launch host mode")
	}
	if runtime.GOOS == "windows" && !isNativeMessagingLaunch([]string{"--parent-window=123"}) {
		t.Fatal("expected Windows parent window arg to launch host mode")
	}
	if isNativeMessagingLaunch([]string{"host"}) {
		t.Fatal("expected CLI subcommand not to be treated as native messaging launch")
	}
	if !isNativeMessagingLaunch([]string{filepath.Join(t.TempDir(), host.NativeHostName+".json"), defaultFirefoxExtensionID}) {
		t.Fatal("expected Firefox native manifest argument to launch host mode")
	}
}

func TestCobraVersionCommand(t *testing.T) {
	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(output.String()); got != version {
		t.Fatalf("expected version %q, got %q", version, got)
	}
}

func TestCobraNoArgsPrintsVersionAndExtensionStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if !strings.Contains(got, "📦 CLI version: "+version) {
		t.Fatalf("expected no-arg output to include CLI version, got %q", got)
	}
	if !strings.Contains(got, "🧩 Browser extension:") {
		t.Fatalf("expected no-arg output to include browser extension status, got %q", got)
	}
	if !strings.Contains(got, "open-browser-use setup") && !strings.Contains(got, "open-browser-use user-tabs") {
		t.Fatalf("expected no-arg output to include a setup or ready next step, got %q", got)
	}
}

func TestCobraVersionFlag(t *testing.T) {
	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"-v"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(output.String()); got != version {
		t.Fatalf("expected version %q, got %q", version, got)
	}
}

func TestNativeManifestDefaultsToStoreExtensionAndStableHostPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	manifest, err := nativeManifest("", "")
	if err != nil {
		t.Fatal(err)
	}
	origins, ok := manifest["allowed_origins"].([]string)
	if !ok || len(origins) != 1 {
		t.Fatalf("expected one allowed origin, got %#v", manifest["allowed_origins"])
	}
	if origins[0] != "chrome-extension://"+defaultChromeExtensionID+"/" {
		t.Fatalf("expected default extension origin, got %q", origins[0])
	}
	hostPath, err := stableNativeHostPath()
	if err != nil {
		t.Fatal(err)
	}
	if manifest["path"] != hostPath {
		t.Fatalf("expected manifest path %q, got %#v", hostPath, manifest["path"])
	}
}

func TestInstallNativeManifestCreatesStableLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stable native host link is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	targetPath := filepath.Join(t.TempDir(), "open-browser-use")
	if err := os.WriteFile(targetPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	manifestPath, err := installNativeManifest("", targetPath, "")
	if err != nil {
		t.Fatal(err)
	}
	hostPath, err := stableNativeHostPath()
	if err != nil {
		t.Fatal(err)
	}
	linkTarget, err := os.Readlink(hostPath)
	if err != nil {
		t.Fatal(err)
	}
	if linkTarget != targetPath {
		t.Fatalf("expected stable link to point at %q, got %q", targetPath, linkTarget)
	}
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["path"] != hostPath {
		t.Fatalf("expected manifest path %q, got %#v", hostPath, manifest["path"])
	}
}

func TestCobraInstallManifestDefaultsToStoreExtension(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stable native host link is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	targetPath := filepath.Join(t.TempDir(), "open-browser-use")
	if err := os.WriteFile(targetPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"install-manifest", "--path", targetPath})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	manifestPath := strings.TrimSpace(output.String())
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatal(err)
	}
	origins, ok := manifest["allowed_origins"].([]any)
	if !ok || len(origins) != 1 {
		t.Fatalf("expected one allowed origin, got %#v", manifest["allowed_origins"])
	}
	if origins[0] != "chrome-extension://"+defaultChromeExtensionID+"/" {
		t.Fatalf("expected default extension origin, got %#v", origins[0])
	}
}

func TestCobraInstallManifestSupportsChromeBeta(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Chrome Beta manifest path test uses macOS browser roots")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	targetPath := filepath.Join(t.TempDir(), "open-browser-use")
	if err := os.WriteFile(targetPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"install-manifest", "--browser", "chrome-beta", "--path", targetPath})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "Library/Application Support/Google/Chrome Beta/NativeMessagingHosts", host.NativeHostName+".json")
	if strings.TrimSpace(output.String()) != want {
		t.Fatalf("expected Chrome Beta manifest path %q, got %q", want, strings.TrimSpace(output.String()))
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatal(err)
	}
}

func TestInstallNativeManifestSupportsZen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stable native host test requires an executable link target")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	targetPath := filepath.Join(t.TempDir(), "open-browser-use")
	if err := os.WriteFile(targetPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	manifestPath, err := installNativeManifestForBrowser("", targetPath, "", "zen")
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(home, ".mozilla/native-messaging-hosts", host.NativeHostName+".json")
	if runtime.GOOS == "darwin" {
		wantPath = filepath.Join(home, "Library/Application Support/Mozilla/NativeMessagingHosts", host.NativeHostName+".json")
	}
	if manifestPath != wantPath {
		t.Fatalf("expected Zen manifest path %q, got %q", wantPath, manifestPath)
	}
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatal(err)
	}
	extensions, ok := manifest["allowed_extensions"].([]any)
	if !ok || len(extensions) != 1 || extensions[0] != defaultFirefoxExtensionID {
		t.Fatalf("expected default Firefox extension id, got %#v", manifest["allowed_extensions"])
	}
	if _, exists := manifest["allowed_origins"]; exists {
		t.Fatalf("did not expect Chromium allowed_origins in Zen manifest: %#v", manifest)
	}
}

func TestInstallChromeExternalExtensionWritesWebStoreHint(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), defaultChromeExtensionID+".json")
	path, err := installChromeExternalExtension("", outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if path != outputPath {
		t.Fatalf("expected output path %q, got %q", outputPath, path)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["external_update_url"] != chromeWebStoreUpdateURL {
		t.Fatalf("expected Chrome Web Store update URL, got %#v", manifest["external_update_url"])
	}
}

func TestCobraSetupWritesNativeAndExternalManifests(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stable native host link is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	targetPath := filepath.Join(t.TempDir(), "open-browser-use")
	if err := os.WriteFile(targetPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	externalPath := filepath.Join(t.TempDir(), defaultChromeExtensionID+".json")

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"setup", "--path", targetPath, "--external-extension-output", externalPath, "--no-open"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "✅ Open Browser Use setup") {
		t.Fatalf("expected setup output to mention native manifest, got %q", output.String())
	}
	if !strings.Contains(output.String(), "Registered native host") {
		t.Fatalf("expected setup output to mention native host registration, got %q", output.String())
	}
	if !strings.Contains(output.String(), "Browser extension") {
		t.Fatalf("expected setup output to mention browser extension status, got %q", output.String())
	}
	if !strings.Contains(output.String(), chromeWebStoreExtensionURL) {
		t.Fatalf("expected setup output to mention Chrome Web Store URL, got %q", output.String())
	}
	if _, err := os.Stat(filepath.Join(home, "Library/Application Support/Google/Chrome/NativeMessagingHosts", host.NativeHostName+".json")); runtime.GOOS == "darwin" && err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(externalPath); err != nil {
		t.Fatal(err)
	}
}

func TestCobraSetupRejectsBitBrowserExternalExtensionPath(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("BitBrowser setup guidance test uses macOS browser roots")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeLocalStateAtRoot(t, bitBrowserRootForTest(home, "abc123"), `{"profile":{"info_cache":{"Default":{"name":"Bit"}}}}`)
	targetPath := filepath.Join(t.TempDir(), "open-browser-use")
	if err := os.WriteFile(targetPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCommand()
	cmd.SetArgs([]string{"setup", "--browser", "abc123", "--path", targetPath, "--no-open"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected BitBrowser setup to require install-manifest path")
	}
	if !strings.Contains(err.Error(), "install-manifest --browser abc123") {
		t.Fatalf("expected BitBrowser install-manifest guidance, got %v", err)
	}
	manifestPath := filepath.Join(bitBrowserRootForTest(home, "abc123"), "NativeMessagingHosts", host.NativeHostName+".json")
	if _, statErr := os.Stat(manifestPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected setup rejection to avoid partial native manifest write, stat err=%v", statErr)
	}
}

func TestCobraSetupBetaUsesProvidedZIP(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stable native host link is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	targetPath := filepath.Join(t.TempDir(), "open-browser-use")
	if err := os.WriteFile(targetPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(t.TempDir(), "open-browser-use-chrome-extension.zip")
	expectedExtensionID, err := extensionIDFromPublicKey(betaExtensionPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTestExtensionZIP(zipPath); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"setup", "beta", "--path", targetPath, "--zip", zipPath, "--no-open"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if !strings.Contains(got, "ZIP:") || !strings.Contains(got, zipPath) {
		t.Fatalf("expected setup beta output to mention install ZIP path, got %q", got)
	}
	if strings.Contains(got, "-manual.zip") {
		t.Fatalf("expected setup beta output to avoid a separate manual ZIP path, got %q", got)
	}
	if !strings.Contains(got, "Extension id: "+expectedExtensionID) {
		t.Fatalf("expected setup beta output to mention unpacked extension id, got %q", got)
	}
	if !strings.Contains(got, "drag in "+zipPath) && !strings.Contains(got, "All set.") {
		t.Fatalf("expected setup beta output to mention manual install or connected status, got %q", got)
	}
	manifestPath := filepath.Join(home, "Library/Application Support/Google/Chrome/NativeMessagingHosts", host.NativeHostName+".json")
	if runtime.GOOS == "linux" {
		manifestPath = filepath.Join(home, ".config/google-chrome/NativeMessagingHosts", host.NativeHostName+".json")
	}
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "chrome-extension://"+expectedExtensionID+"/") {
		t.Fatalf("expected native manifest to allow unpacked extension id, got %s", payload)
	}
	unpackedPath, err := defaultUnpackedExtensionDir()
	if err != nil {
		t.Fatal(err)
	}
	unpackedManifest, err := os.ReadFile(filepath.Join(unpackedPath, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(unpackedManifest), betaExtensionPublicKey) {
		t.Fatalf("expected unpacked manifest to include stable key, got %s", unpackedManifest)
	}
	installManifest, err := readManifestFromZIP(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(installManifest), betaExtensionPublicKey) {
		t.Fatalf("expected install ZIP manifest to include stable key, got %s", installManifest)
	}
}

func TestDetectInstalledChromeExtensionFromProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	profileRoot := filepath.Join(home, "Library/Application Support/Google/Chrome/Default/Extensions", defaultChromeExtensionID, "0.1.10")
	if runtime.GOOS == "linux" {
		profileRoot = filepath.Join(home, ".config/google-chrome/Default/Extensions", defaultChromeExtensionID, "0.1.10")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Chrome profile detection is not implemented on windows")
	}
	if err := os.MkdirAll(profileRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileRoot, "manifest.json"), []byte(`{"version":"0.1.10"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	detected, ok := detectInstalledChromeExtension()
	if !ok {
		t.Fatal("expected installed extension to be detected")
	}
	if detected.ExtensionID != defaultChromeExtensionID {
		t.Fatalf("expected extension id %q, got %q", defaultChromeExtensionID, detected.ExtensionID)
	}
	if detected.Version != "0.1.10" {
		t.Fatalf("expected version 0.1.10, got %q", detected.Version)
	}
}

func writeChromeExtensionManifest(t *testing.T, home, profileDir, extensionVersion string) {
	t.Helper()
	base := filepath.Join(home, "Library/Application Support/Google/Chrome")
	if runtime.GOOS == "linux" {
		base = filepath.Join(home, ".config/google-chrome")
	}
	writeExtensionManifestAtRoot(t, base, profileDir, defaultChromeExtensionID, extensionVersion)
}

func writeExtensionManifestAtRoot(t *testing.T, root, profileDir, extensionID, extensionVersion string) {
	t.Helper()
	dir := filepath.Join(root, profileDir, "Extensions", extensionID, extensionVersion)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"version":%q}`, extensionVersion)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
}

func chromeRootForTest(home string) string {
	if runtime.GOOS == "linux" {
		return filepath.Join(home, ".config/google-chrome")
	}
	return filepath.Join(home, "Library/Application Support/Google/Chrome")
}

func chromeBetaRootForTest(home string) string {
	return filepath.Join(home, "Library/Application Support/Google/Chrome Beta")
}

func bitBrowserRootForTest(home, instance string) string {
	return filepath.Join(home, "Library/Application Support/BitBrowser/BrowserCache", instance)
}

func zenRootForTest(home string) string {
	if runtime.GOOS == "linux" {
		return filepath.Join(home, ".config/zen")
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "AppData/Roaming/zen")
	}
	return filepath.Join(home, "Library/Application Support/zen")
}

func writeZenExtensionRegistry(t *testing.T, home, profileDir, profileName, extensionVersion string) {
	t.Helper()
	root := zenRootForTest(home)
	profilePath := filepath.Join(root, "Profiles", profileDir)
	if err := os.MkdirAll(profilePath, 0o700); err != nil {
		t.Fatal(err)
	}
	profilesINI := fmt.Sprintf("[Profile0]\nName=%s\nIsRelative=1\nPath=Profiles/%s\n", profileName, profileDir)
	if err := os.WriteFile(filepath.Join(root, "profiles.ini"), []byte(profilesINI), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := fmt.Sprintf(`{"addons":[{"id":%q,"version":%q,"path":"extensions/%s.xpi","active":true}]}`, defaultFirefoxExtensionID, extensionVersion, defaultFirefoxExtensionID)
	if err := os.WriteFile(filepath.Join(profilePath, "extensions.json"), []byte(registry), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeLocalStateAtRoot(t *testing.T, root string, payload string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Local State"), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeExtensionStorageAtRoot(t *testing.T, root, profileDir, extensionID, payload string) {
	t.Helper()
	storageDir := filepath.Join(root, profileDir, "Local Extension Settings", extensionID)
	if err := os.MkdirAll(storageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "000003.log"), []byte("garbage prefix\x00\x01extensionInstanceId\""+payload+"\"\x00more garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeChromeBetaExtensionManifest(t *testing.T, home, profileDir, extensionVersion string) {
	t.Helper()
	writeExtensionManifestAtRoot(t, chromeBetaRootForTest(home), profileDir, defaultChromeExtensionID, extensionVersion)
}

func writeBitBrowserExtensionManifest(t *testing.T, home, instance, profileDir, extensionVersion string) {
	t.Helper()
	writeExtensionManifestAtRoot(t, bitBrowserRootForTest(home, instance), profileDir, defaultChromeExtensionID, extensionVersion)
}

func writeChromeLocalState(t *testing.T, home string, payload string) {
	t.Helper()
	writeLocalStateAtRoot(t, chromeRootForTest(home), payload)
}

func writeChromeUnpackedExtension(t *testing.T, home, profileDir, extensionID, extensionVersion string) {
	t.Helper()
	base := filepath.Join(home, "Library/Application Support/Google/Chrome")
	if runtime.GOOS == "linux" {
		base = filepath.Join(home, ".config/google-chrome")
	}
	unpackedDir := filepath.Join(base, profileDir, "UnpackedExtensions", "open-browser-use-chrome-extension-"+extensionVersion+"_test")
	if err := os.MkdirAll(unpackedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"version":%q}`, extensionVersion)
	if err := os.WriteFile(filepath.Join(unpackedDir, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	profileRoot := filepath.Join(base, profileDir)
	if err := os.MkdirAll(profileRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	prefs := map[string]any{
		"extensions": map[string]any{
			"settings": map[string]any{
				extensionID: map[string]any{
					"path": unpackedDir,
				},
			},
		},
	}
	payload, err := json.Marshal(prefs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileRoot, "Secure Preferences"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestListInstalledChromeProfilesDetectsUnpackedExtension(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Chrome profile detection is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	betaID, err := extensionIDFromPublicKey(betaExtensionPublicKey)
	if err != nil {
		t.Fatalf("could not derive beta extension id: %v", err)
	}
	writeChromeUnpackedExtension(t, home, "Profile 1", betaID, "0.1.29")
	writeChromeLocalState(t, home, `{"profile":{"info_cache":{"Profile 1":{"name":"Beta"}}}}`)

	profiles, err := listInstalledChromeProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %+v", profiles)
	}
	if profiles[0].ExtensionID != betaID {
		t.Errorf("expected beta extension id %q, got %q", betaID, profiles[0].ExtensionID)
	}
	if profiles[0].Version != "0.1.29" {
		t.Errorf("expected version 0.1.29, got %q", profiles[0].Version)
	}
}

func TestListInstalledChromeProfilesPrefersHigherVersionAcrossCRXAndUnpacked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Chrome profile detection is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeChromeExtensionManifest(t, home, "Default", "0.1.10")
	betaID, err := extensionIDFromPublicKey(betaExtensionPublicKey)
	if err != nil {
		t.Fatalf("could not derive beta extension id: %v", err)
	}
	writeChromeUnpackedExtension(t, home, "Default", betaID, "0.2.0")

	profiles, err := listInstalledChromeProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %+v", profiles)
	}
	if profiles[0].Version != "0.2.0" {
		t.Errorf("expected newer beta version 0.2.0 to win, got %q", profiles[0].Version)
	}
}

func TestListInstalledChromeProfilesSingleProfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Chrome profile detection is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeChromeExtensionManifest(t, home, "Default", "0.1.10")
	writeChromeLocalState(t, home, `{"profile":{"info_cache":{"Default":{"name":"Eva"}}}}`)

	profiles, err := listInstalledChromeProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d (%+v)", len(profiles), profiles)
	}
	got := profiles[0]
	if got.Directory != "Default" {
		t.Errorf("expected directory Default, got %q", got.Directory)
	}
	if got.DisplayName != "Eva" {
		t.Errorf("expected display name Eva, got %q", got.DisplayName)
	}
	if got.Version != "0.1.10" {
		t.Errorf("expected version 0.1.10, got %q", got.Version)
	}
	if got.ExtensionID != defaultChromeExtensionID {
		t.Errorf("expected default extension id, got %q", got.ExtensionID)
	}
}

func TestListInstalledChromeProfilesMultipleProfilesSorted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Chrome profile detection is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeChromeExtensionManifest(t, home, "Profile 10", "0.1.30")
	writeChromeExtensionManifest(t, home, "Profile 2", "0.1.20")
	writeChromeExtensionManifest(t, home, "Default", "0.1.10")
	writeChromeLocalState(t, home, `{"profile":{"info_cache":{
		"Default":{"name":"Eva"},
		"Profile 2":{"name":"Work"},
		"Profile 10":{"name":"测试 🧪"}
	}}}`)

	profiles, err := listInstalledChromeProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d (%+v)", len(profiles), profiles)
	}
	wantOrder := []string{"Default", "Profile 2", "Profile 10"}
	for i, dir := range wantOrder {
		if profiles[i].Directory != dir {
			t.Errorf("profile[%d]: expected directory %q, got %q", i, dir, profiles[i].Directory)
		}
	}
	if profiles[2].DisplayName != "测试 🧪" {
		t.Errorf("expected unicode display name preserved, got %q", profiles[2].DisplayName)
	}
}

func TestListInstalledChromeProfilesIncludesSupportedBrowsers(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("multi-browser discovery test uses macOS browser roots")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeChromeExtensionManifest(t, home, "Default", "0.1.36")
	writeChromeLocalState(t, home, `{"profile":{"info_cache":{"Default":{"name":"Stable"}}}}`)
	writeChromeBetaExtensionManifest(t, home, "Default", "0.1.36")
	writeLocalStateAtRoot(t, chromeBetaRootForTest(home), `{"profile":{"info_cache":{"Default":{"name":"Beta"}}}}`)
	writeBitBrowserExtensionManifest(t, home, "abc123", "Default", "0.1.36")
	writeLocalStateAtRoot(t, bitBrowserRootForTest(home, "abc123"), `{"profile":{"info_cache":{"Default":{"name":"Bit"}}}}`)

	profiles, err := listInstalledChromeProfiles()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]installedChromeProfile{}
	for _, profile := range profiles {
		got[profile.Target] = profile
	}
	for _, target := range []string{"chrome:Default", "chrome-beta:Default", "bitbrowser:abc123:Default"} {
		if _, ok := got[target]; !ok {
			t.Fatalf("expected target %q in profiles: %+v", target, profiles)
		}
	}
	if got["chrome-beta:Default"].BrowserName != "Google Chrome Beta" {
		t.Fatalf("expected Chrome Beta browser name, got %+v", got["chrome-beta:Default"])
	}
	if got["bitbrowser:abc123:Default"].BrowserInstance != "abc123" {
		t.Fatalf("expected BitBrowser instance, got %+v", got["bitbrowser:abc123:Default"])
	}
}

func TestListInstalledZenProfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeZenExtensionRegistry(t, home, "abc123.default-release", "Personal", "0.1.40")

	profiles, err := listInstalledChromeProfiles()
	if err != nil {
		t.Fatal(err)
	}
	var zen *installedChromeProfile
	for i := range profiles {
		if profiles[i].Browser == "zen" {
			zen = &profiles[i]
			break
		}
	}
	if zen == nil {
		t.Fatalf("expected Zen profile in %+v", profiles)
	}
	if zen.BrowserName != "Zen Browser" || zen.Directory != "abc123.default-release" || zen.DisplayName != "Personal" {
		t.Fatalf("unexpected Zen profile: %+v", *zen)
	}
	if zen.ExtensionID != defaultFirefoxExtensionID || zen.Target != "zen:abc123.default-release" {
		t.Fatalf("unexpected Zen extension routing metadata: %+v", *zen)
	}
}

func TestFirefoxProfilePathsSupportsDirectZenLayout(t *testing.T) {
	root := t.TempDir()
	profileDir := filepath.Join(root, "d7sqxvvc.Default (release)")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}
	profilesINI := "[Profile0]\nName=Default (release)\nIsRelative=1\nPath=d7sqxvvc.Default (release)\n"
	if err := os.WriteFile(filepath.Join(root, "profiles.ini"), []byte(profilesINI), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := firefoxProfilePaths(root)
	if len(paths) != 1 || paths[0] != profileDir {
		t.Fatalf("expected direct Zen profile path %q, got %+v", profileDir, paths)
	}
}

func TestResolveZenProfileFromExtensionOrigin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	profileDir := "abc123.default-release"
	writeZenExtensionRegistry(t, home, profileDir, "Personal", "0.1.40")
	prefs := `user_pref("extensions.webextensions.uuids", "{\"open-browser-use@ifuryst.com\":\"2b27a953-2f07-4ff7-a77d-8616c81a3419\"}");`
	if err := os.WriteFile(filepath.Join(zenRootForTest(home), "Profiles", profileDir, "prefs.js"), []byte(prefs), 0o600); err != nil {
		t.Fatal(err)
	}

	browserID, browserName, _, dir, name, ok := resolveProfileForExtension(
		defaultFirefoxExtensionID,
		"instance-not-yet-flushed-to-disk",
		"moz-extension://2b27a953-2f07-4ff7-a77d-8616c81a3419/",
	)
	if !ok || browserID != "zen" || browserName != "Zen Browser" || dir != profileDir || name != "Personal" {
		t.Fatalf("unexpected Zen origin resolution: browser=%q name=%q dir=%q profile=%q ok=%v", browserID, browserName, dir, name, ok)
	}
}

func TestListInstalledChromeProfilesSkipsProfilesWithoutExtension(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Chrome profile detection is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeChromeExtensionManifest(t, home, "Default", "0.1.10")
	// Create a profile directory with no extension installed.
	base := filepath.Join(home, "Library/Application Support/Google/Chrome")
	if runtime.GOOS == "linux" {
		base = filepath.Join(home, ".config/google-chrome")
	}
	if err := os.MkdirAll(filepath.Join(base, "Profile 1"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeChromeLocalState(t, home, `{"profile":{"info_cache":{"Default":{"name":"Eva"},"Profile 1":{"name":"Empty"}}}}`)

	profiles, err := listInstalledChromeProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected only the Default profile, got %+v", profiles)
	}
	if profiles[0].Directory != "Default" {
		t.Errorf("expected Default, got %q", profiles[0].Directory)
	}
}

func TestListInstalledChromeProfilesMissingLocalState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Chrome profile detection is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeChromeExtensionManifest(t, home, "Default", "0.1.10")

	profiles, err := listInstalledChromeProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %+v", profiles)
	}
	if profiles[0].DisplayName != "" {
		t.Errorf("expected empty display name when Local State missing, got %q", profiles[0].DisplayName)
	}
}

func TestListInstalledChromeProfilesCorruptLocalState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Chrome profile detection is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeChromeExtensionManifest(t, home, "Default", "0.1.10")
	writeChromeLocalState(t, home, `{this is not json`)

	profiles, err := listInstalledChromeProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %+v", profiles)
	}
	if profiles[0].DisplayName != "" {
		t.Errorf("expected display name to fall back to empty on corrupt Local State, got %q", profiles[0].DisplayName)
	}
}

func TestProfilesCommandJSONOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Chrome profile detection is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeChromeExtensionManifest(t, home, "Default", "0.1.10")
	writeChromeExtensionManifest(t, home, "Profile 1", "0.1.10")
	writeChromeLocalState(t, home, `{"profile":{"info_cache":{"Default":{"name":"Eva"},"Profile 1":{"name":"Work"}}}}`)

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"profiles", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got []installedChromeProfile
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("expected JSON output, got %q: %v", output.String(), err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 profiles in JSON output, got %+v", got)
	}
	if got[0].Directory != "Default" || got[1].Directory != "Profile 1" {
		t.Errorf("unexpected directories: %+v", got)
	}
}

func TestProfilesJSONOutputRemainsValidWithUnresolvedHost(t *testing.T) {
	profiles := []installedChromeProfile{{
		Directory:   "Default",
		DisplayName: "Eva",
		ExtensionID: defaultChromeExtensionID,
		Version:     "0.1.10",
		Source:      "/tmp/manifest.json",
	}}
	connected := []connectedProfileInfo{{
		SocketPath: "/tmp/open-browser-use/unresolved.sock",
		InstanceID: "11111111-aaaa-bbbb-cccc-1234567890ab",
	}}

	var output bytes.Buffer
	if err := renderProfilesList(&output, profiles, connected, true, true); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("expected strict JSON output, got %q: %v", output.String(), err)
	}
	if len(got) != 2 {
		t.Fatalf("expected installed profile and unresolved connected host rows, got %+v", got)
	}
	if _, ok := got[0]["connected"]; ok {
		t.Fatalf("unresolved host should not mark installed profile connected: %+v", got[0])
	}
	if got[1]["connected"] != true || got[1]["instanceId"] != connected[0].InstanceID || got[1]["socketPath"] != connected[0].SocketPath {
		t.Fatalf("unresolved connected host must remain discoverable: %+v", got[1])
	}
}

func TestProfilesCommandHumanOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Chrome profile detection is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeChromeExtensionManifest(t, home, "Default", "0.1.10")
	writeChromeLocalState(t, home, `{"profile":{"info_cache":{"Default":{"name":"Eva"}}}}`)

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"profiles"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{"DIRECTORY", "DISPLAY NAME", "VERSION", "Default", "Eva", "0.1.10"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, text)
		}
	}
}

func TestProfilesCommandEmptyOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Chrome profile detection is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"profiles"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "No browser profiles") {
		t.Errorf("expected empty notice, got:\n%s", output.String())
	}
}

func writeChromeExtensionStorage(t *testing.T, home, profileDir, extensionID, payload string) {
	t.Helper()
	base := filepath.Join(home, "Library/Application Support/Google/Chrome")
	if runtime.GOOS == "linux" {
		base = filepath.Join(home, ".config/google-chrome")
	}
	storageDir := filepath.Join(base, profileDir, "Local Extension Settings", extensionID)
	if err := os.MkdirAll(storageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Mimic a LevelDB write-ahead log: arbitrary bytes that contain the UUID
	// somewhere inside. Our resolver byte-greps so format doesn't matter.
	if err := os.WriteFile(filepath.Join(storageDir, "000003.log"), []byte("garbage prefix\x00\x01extensionInstanceId\""+payload+"\"\x00more garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveProfileForInstanceIDHit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Chrome profile detection is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeChromeLocalState(t, home, `{"profile":{"info_cache":{"Default":{"name":"Eva"},"Profile 1":{"name":"Work"}}}}`)
	writeChromeExtensionStorage(t, home, "Default", defaultChromeExtensionID, "11111111-aaaa-bbbb-cccc-1234567890ab")
	writeChromeExtensionStorage(t, home, "Profile 1", defaultChromeExtensionID, "22222222-aaaa-bbbb-cccc-1234567890ab")

	browserID, _, _, dir, name, ok := resolveProfileForInstanceID(defaultChromeExtensionID, "22222222-aaaa-bbbb-cccc-1234567890ab")
	if !ok {
		t.Fatal("expected to resolve instance to a profile")
	}
	if browserID != "chrome" {
		t.Errorf("expected browser chrome, got %q", browserID)
	}
	if dir != "Profile 1" {
		t.Errorf("expected directory Profile 1, got %q", dir)
	}
	if name != "Work" {
		t.Errorf("expected display name Work, got %q", name)
	}
}

func TestResolveProfileForInstanceIDChromeBeta(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Chrome Beta discovery test uses macOS browser roots")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeLocalStateAtRoot(t, chromeBetaRootForTest(home), `{"profile":{"info_cache":{"Default":{"name":"Beta"}}}}`)
	writeExtensionStorageAtRoot(t, chromeBetaRootForTest(home), "Default", defaultChromeExtensionID, "33333333-aaaa-bbbb-cccc-1234567890ab")

	browserID, browserName, _, dir, name, ok := resolveProfileForInstanceID(defaultChromeExtensionID, "33333333-aaaa-bbbb-cccc-1234567890ab")
	if !ok {
		t.Fatal("expected to resolve Chrome Beta instance")
	}
	if browserID != "chrome-beta" || browserName != "Google Chrome Beta" || dir != "Default" || name != "Beta" {
		t.Fatalf("unexpected Chrome Beta resolution: browser=%q name=%q dir=%q profile=%q", browserID, browserName, dir, name)
	}
}

func TestResolveProfileForInstanceIDMiss(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Chrome profile detection is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeChromeExtensionStorage(t, home, "Default", defaultChromeExtensionID, "11111111-aaaa-bbbb-cccc-1234567890ab")

	if _, _, _, _, _, ok := resolveProfileForInstanceID(defaultChromeExtensionID, "ffffffff-ffff-ffff-ffff-ffffffffffff"); ok {
		t.Fatal("expected no resolution for unknown UUID")
	}
	if _, _, _, _, _, ok := resolveProfileForInstanceID(defaultChromeExtensionID, ""); ok {
		t.Fatal("expected empty instance to not resolve")
	}
	if _, _, _, _, _, ok := resolveProfileForInstanceID("", "11111111-aaaa-bbbb-cccc-1234567890ab"); ok {
		t.Fatal("expected empty extension id to not resolve")
	}
}

func TestProfileSelectorMatches(t *testing.T) {
	info := connectedProfileInfo{
		Directory:   "Profile 1",
		DisplayName: "Work",
		InstanceID:  "11111111-aaaa-bbbb-cccc-1234567890ab",
	}
	cases := []struct {
		selector string
		want     bool
	}{
		{"", true},
		{"Profile 1", true},
		{"profile 1", true},
		{"Work", true},
		{"work", true},
		{"11111111-aaaa-bbbb-cccc-1234567890ab", true},
		{"Eva", false},
		{"Default", false},
	}
	for _, tc := range cases {
		if got := profileSelectorMatches(tc.selector, info); got != tc.want {
			t.Errorf("selector %q -> %v, want %v", tc.selector, got, tc.want)
		}
	}
}

func TestBrowserSelectorMatches(t *testing.T) {
	info := connectedProfileInfo{
		Browser:         "bitbrowser",
		BrowserName:     "BitBrowser",
		BrowserInstance: "abc123",
		Directory:       "Default",
		Target:          "bitbrowser:abc123:Default",
	}
	cases := []struct {
		selector string
		want     bool
	}{
		{"", true},
		{"bitbrowser", true},
		{"BitBrowser", true},
		{"abc123", true},
		{"bitbrowser:abc123:Default", true},
		{"chrome", false},
	}
	for _, tc := range cases {
		if got := browserSelectorMatches(tc.selector, info); got != tc.want {
			t.Errorf("selector %q -> %v, want %v", tc.selector, got, tc.want)
		}
	}
}

func TestProfileSelectorMatchesUnresolved(t *testing.T) {
	info := connectedProfileInfo{InstanceID: "abc"}
	if !profileSelectorMatches("ABC", info) {
		t.Error("expected case-insensitive instance id match")
	}
	if profileSelectorMatches("Default", info) {
		t.Error("expected directory mismatch when info has no directory")
	}
}

func TestConnectedProfileFromInfo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Chrome profile detection is not implemented on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeChromeLocalState(t, home, `{"profile":{"info_cache":{"Profile 5":{"name":"Test"}}}}`)
	writeChromeExtensionStorage(t, home, "Profile 5", defaultChromeExtensionID, "abcd1234-aaaa-bbbb-cccc-1234567890ab")

	payload := map[string]any{
		"result": map[string]any{
			"metadata": map[string]any{
				"extensionId":         defaultChromeExtensionID,
				"extensionInstanceId": "abcd1234-aaaa-bbbb-cccc-1234567890ab",
			},
		},
	}
	got := connectedProfileFromInfo("/tmp/some.sock", payload)
	if got.SocketPath != "/tmp/some.sock" || got.Browser != "chrome" || got.Target != "chrome:Profile 5" || got.Directory != "Profile 5" || got.DisplayName != "Test" {
		t.Errorf("unexpected resolution: %+v", got)
	}
}

func TestChromeProfileSortKey(t *testing.T) {
	cases := []struct {
		dirs []string
		want []string
	}{
		{
			dirs: []string{"Profile 10", "Default", "Profile 2", "Profile 1"},
			want: []string{"Default", "Profile 1", "Profile 2", "Profile 10"},
		},
		{
			dirs: []string{"Profile Foo", "Default", "Profile 1"},
			want: []string{"Default", "Profile 1", "Profile Foo"},
		},
	}
	for _, tc := range cases {
		got := append([]string(nil), tc.dirs...)
		sort.SliceStable(got, func(i, j int) bool {
			return chromeProfileSortKey(got[i]) < chromeProfileSortKey(got[j])
		})
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("sort %v => %v, want %v", tc.dirs, got, tc.want)
				break
			}
		}
	}
}

func TestCompareChromeVersions(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  int
	}{
		{left: "0.1.15", right: "0.1.11", want: 1},
		{left: "0.1.15", right: "0.1.15", want: 0},
		{left: "0.1.11", right: "0.1.15", want: -1},
		{left: "0.1", right: "0.1.0", want: 0},
	}
	for _, test := range tests {
		got := compareChromeVersions(test.left, test.right)
		if got != test.want {
			t.Fatalf("compareChromeVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}

func TestBrowserExtensionStatusSummaries(t *testing.T) {
	ready := browserExtensionStatus{
		Installed:       true,
		Reachable:       true,
		Version:         version,
		ExpectedVersion: version,
	}
	if got := ready.summary(); !strings.Contains(got, "Ready") || !strings.Contains(got, version) {
		t.Fatalf("expected ready summary with version, got %q", got)
	}

	outdated := browserExtensionStatus{
		Installed:       true,
		Version:         "0.1.0",
		ExpectedVersion: version,
		UpgradeCommand:  "open-browser-use setup",
	}
	if got := outdated.summary(); !strings.Contains(got, "CLI expects v"+version) || !strings.Contains(got, "open-browser-use setup") {
		t.Fatalf("expected upgrade summary with command, got %q", got)
	}
}

func TestShouldOpenManualSetup(t *testing.T) {
	tests := []struct {
		name   string
		status browserExtensionStatus
		noOpen bool
		want   bool
	}{
		{
			name: "missing extension opens guidance",
			status: browserExtensionStatus{
				ExpectedVersion: version,
			},
			want: true,
		},
		{
			name: "outdated extension opens guidance",
			status: browserExtensionStatus{
				Installed:       true,
				Version:         "0.1.0",
				ExpectedVersion: version,
			},
			want: true,
		},
		{
			name: "current installed extension skips guidance even when disconnected",
			status: browserExtensionStatus{
				Installed:       true,
				Version:         version,
				ExpectedVersion: version,
			},
		},
		{
			name: "no-open suppresses guidance",
			status: browserExtensionStatus{
				ExpectedVersion: version,
			},
			noOpen: true,
		},
	}
	for _, test := range tests {
		got := shouldOpenManualSetup(test.status, test.noOpen)
		if got != test.want {
			t.Fatalf("%s: shouldOpenManualSetup() = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestShouldOpenStoreSetup(t *testing.T) {
	current := browserExtensionStatus{
		Installed:       true,
		Version:         version,
		ExpectedVersion: version,
	}
	if shouldOpenStoreSetup(current, false) {
		t.Fatal("expected current store extension to skip store page")
	}
	missing := browserExtensionStatus{ExpectedVersion: version}
	if !shouldOpenStoreSetup(missing, false) {
		t.Fatal("expected missing store extension to open store page")
	}
	if shouldOpenStoreSetup(missing, true) {
		t.Fatal("expected --no-open to suppress store page")
	}
}

func TestCobraUnknownCommand(t *testing.T) {
	cmd := newRootCommand()
	cmd.SetArgs([]string{"does-not-exist"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected unknown command to fail")
	}
}

func TestInvokeRemovesStaleActiveSocketRecord(t *testing.T) {
	socketDir := t.TempDir()
	socketPath := filepath.Join(socketDir, "missing.sock")
	if err := host.WriteActiveSocketRecord(socketDir, socketPath); err != nil {
		t.Fatal(err)
	}

	_, err := invoke("", socketDir, "getInfo", map[string]any{}, 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected stale active socket to fail")
	}
	if !strings.Contains(err.Error(), "removed stale registry entry") {
		t.Fatalf("expected stale registry cleanup error, got %v", err)
	}
	if _, err := host.ReadActiveSocketRecord(socketDir); err == nil {
		t.Fatal("expected stale active socket record to be removed")
	}
}

func TestInvokeScansSocketDirWhenActiveRecordMissing(t *testing.T) {
	socketDir, err := os.MkdirTemp("/tmp", "obu-socket-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(socketDir)
	stalePath := filepath.Join(socketDir, "stale.sock")
	staleListener, err := net.Listen("unix", stalePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := staleListener.Close(); err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(socketDir, "live.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()

		var request map[string]any
		if err := wire.ReadJSON(conn, &request); err != nil {
			serverDone <- err
			return
		}
		if request["method"] != "getInfo" {
			serverDone <- fmt.Errorf("expected getInfo request, got %#v", request["method"])
			return
		}
		if err := wire.WriteJSON(conn, map[string]any{
			"jsonrpc": "2.0",
			"id":      request["id"],
			"result":  map[string]any{"version": version},
		}); err != nil {
			serverDone <- err
			return
		}
		serverDone <- nil
	}()

	response, err := invoke("", socketDir, "getInfo", map[string]any{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, _ := response["result"].(map[string]any)
	if result["version"] != version {
		t.Fatalf("expected scanned socket response, got %#v", response)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	record, err := host.ReadActiveSocketRecord(socketDir)
	if err != nil {
		t.Fatal(err)
	}
	if record.SocketPath != socketPath {
		t.Fatalf("expected active socket record to be repaired to %q, got %#v", socketPath, record)
	}
	if record.PID != 0 {
		t.Fatalf("expected repaired active socket pid to be unknown, got %d", record.PID)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("expected stale socket file to be removed after fallback success, got %v", err)
	}
}

func TestInvokeCleansStaleSocketFilesDuringScan(t *testing.T) {
	socketDir, err := os.MkdirTemp("/tmp", "obu-socket-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(socketDir)

	stalePath := filepath.Join(socketDir, "stale.sock")
	staleListener, err := net.Listen("unix", stalePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := staleListener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := host.WriteActiveSocketRecord(socketDir, stalePath); err != nil {
		t.Fatal(err)
	}

	livePath := filepath.Join(socketDir, "live.sock")
	listener, err := net.Listen("unix", livePath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()

		var request map[string]any
		if err := wire.ReadJSON(conn, &request); err != nil {
			serverDone <- err
			return
		}
		if err := wire.WriteJSON(conn, map[string]any{
			"jsonrpc": "2.0",
			"id":      request["id"],
			"result":  map[string]any{"version": version},
		}); err != nil {
			serverDone <- err
			return
		}
		serverDone <- nil
	}()

	if _, err := invoke("", socketDir, "getInfo", map[string]any{}, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("expected stale socket file to be removed, got %v", err)
	}
	record, err := host.ReadActiveSocketRecord(socketDir)
	if err != nil {
		t.Fatal(err)
	}
	if record.SocketPath != livePath {
		t.Fatalf("expected active socket record to point to live socket %q, got %#v", livePath, record)
	}
}

func TestPickSocketForBrowserRepairsActiveRecord(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("multi-browser socket routing test uses macOS browser roots")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeLocalStateAtRoot(t, chromeBetaRootForTest(home), `{"profile":{"info_cache":{"Default":{"name":"Beta"}}}}`)
	writeExtensionStorageAtRoot(t, chromeBetaRootForTest(home), "Default", defaultChromeExtensionID, "beta-instance")

	socketDir, err := os.MkdirTemp("/tmp", "obu-browser-route-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(socketDir)
	socketPath := filepath.Join(socketDir, "beta.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverDone := make(chan error, 1)
	go serveFakeGetInfo(listener, "beta-instance", serverDone)

	conn, info, _, err := pickSocketForProfile(socketDir, "chrome-beta", "Default", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if info.Browser != "chrome-beta" || info.Target != "chrome-beta:Default" {
		t.Fatalf("expected Chrome Beta target, got %+v", info)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	record, err := host.ReadActiveSocketRecord(socketDir)
	if err != nil {
		t.Fatal(err)
	}
	if record.SocketPath != socketPath {
		t.Fatalf("expected active record repaired to %q, got %#v", socketPath, record)
	}
}

func serveFakeGetInfo(listener net.Listener, instanceID string, done chan<- error) {
	conn, err := listener.Accept()
	if err != nil {
		done <- err
		return
	}
	defer conn.Close()
	var request map[string]any
	if err := wire.ReadJSON(conn, &request); err != nil {
		done <- err
		return
	}
	if err := wire.WriteJSON(conn, map[string]any{
		"jsonrpc": "2.0",
		"id":      request["id"],
		"result": map[string]any{
			"version": version,
			"metadata": map[string]any{
				"extensionId":         defaultChromeExtensionID,
				"extensionInstanceId": instanceID,
			},
		},
	}); err != nil {
		done <- err
		return
	}
	done <- nil
}

func TestCobraRunActionPlan(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "obu.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	requests := make(chan map[string]any, 16)
	serverDone := make(chan error, 1)
	go func() {
		defer close(requests)
		var sessionID string
		var turnID string
		for count := 0; count < 10; count++ {
			conn, err := listener.Accept()
			if err != nil {
				serverDone <- err
				return
			}
			var request map[string]any
			if err := wire.ReadJSON(conn, &request); err != nil {
				_ = conn.Close()
				serverDone <- err
				return
			}
			params, _ := request["params"].(map[string]any)
			if sessionID == "" {
				sessionID, _ = params["session_id"].(string)
				turnID, _ = params["turn_id"].(string)
			}
			if params["session_id"] != sessionID || params["turn_id"] != turnID {
				_ = conn.Close()
				serverDone <- errors.New("run action requests did not share session and turn")
				return
			}
			requests <- request

			method, _ := request["method"].(string)
			cdpMethod, _ := params["method"].(string)
			result := map[string]any{}
			switch {
			case method == "createTab":
				result = map[string]any{"id": 123}
			case method == "executeCdp" && cdpMethod == "Runtime.evaluate":
				commandParams, _ := params["commandParams"].(map[string]any)
				expression, _ := commandParams["expression"].(string)
				if expression == "document.readyState" {
					result = map[string]any{"result": map[string]any{"value": "interactive"}}
				} else {
					result = map[string]any{"result": map[string]any{"value": map[string]any{
						"title":      "Browser Use Docs",
						"url":        "https://docs.browser-use.com",
						"readyState": "interactive",
						"text":       "Docs",
					}}}
				}
			}
			if err := wire.WriteJSON(conn, map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result":  result,
			}); err != nil {
				_ = conn.Close()
				serverDone <- err
				return
			}
			_ = conn.Close()
		}
		serverDone <- nil
	}()

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{
		"run",
		"--socket", socketPath,
		"--timeout", "100ms",
		"-c", `
name-session "Docs scan - OBU"
open-tab https://docs.browser-use.com
wait-load domcontentloaded
page-info
finalize-tabs []
`,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}

	var got actionRunOutput
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.CurrentTabID != 123 {
		t.Fatalf("expected current tab id 123, got %d", got.CurrentTabID)
	}
	if len(got.Steps) != 5 {
		t.Fatalf("expected 5 action steps, got %d", len(got.Steps))
	}
	if got.Steps[1].Action != "open-tab" || got.Steps[1].TabID != 123 {
		t.Fatalf("expected open-tab step to capture tab id, got %#v", got.Steps[1])
	}

	var methods []string
	var sessions []string
	for request := range requests {
		method, _ := request["method"].(string)
		params, _ := request["params"].(map[string]any)
		sessionID, _ := params["session_id"].(string)
		sessions = append(sessions, sessionID)
		if cdpMethod, _ := params["method"].(string); cdpMethod != "" {
			method += ":" + cdpMethod
		}
		methods = append(methods, method)
	}
	want := []string{
		"nameSession",
		"createTab",
		"attach",
		"executeCdp:Page.navigate",
		"attach",
		"executeCdp:Page.enable",
		"executeCdp:Runtime.evaluate",
		"attach",
		"executeCdp:Runtime.evaluate",
		"finalizeTabs",
	}
	if strings.Join(methods, ",") != strings.Join(want, ",") {
		t.Fatalf("expected methods %v, got %v", want, methods)
	}
	for _, sessionID := range sessions {
		if sessionID != defaultCLISessionID {
			t.Fatalf("expected run action plan to use CLI session %q, got %q", defaultCLISessionID, sessionID)
		}
	}
}

func TestInvokeWithOptionsUsesSessionID(t *testing.T) {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("obu-test-%d.sock", time.Now().UnixNano()))
	defer os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	requests := make(chan map[string]any, 1)
	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		var request map[string]any
		if err := wire.ReadJSON(conn, &request); err != nil {
			serverDone <- err
			return
		}
		requests <- request
		if err := wire.WriteJSON(conn, map[string]any{
			"jsonrpc": "2.0",
			"id":      request["id"],
			"result":  []any{},
		}); err != nil {
			serverDone <- err
			return
		}
		serverDone <- nil
	}()

	if _, err := invokeWithOptions(socketOptions{
		socketPath: socketPath,
		timeout:    time.Second,
		sessionID:  "custom-cli-session",
	}, "getTabs", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	request := <-requests
	params, _ := request["params"].(map[string]any)
	if params["session_id"] != "custom-cli-session" {
		t.Fatalf("expected custom session id, got %#v", params["session_id"])
	}
}

func testCRX(crxID []byte) []byte {
	signedHeaderData := testProtoBytes(1, crxID)
	header := testProtoBytes(10000, signedHeaderData)
	prefix := make([]byte, 12)
	copy(prefix[0:4], "Cr24")
	prefix[4] = 3
	prefix[8] = byte(len(header))
	prefix[9] = byte(len(header) >> 8)
	prefix[10] = byte(len(header) >> 16)
	prefix[11] = byte(len(header) >> 24)
	return append(append(prefix, header...), []byte("zip")...)
}

func testProtoBytes(fieldNumber uint64, value []byte) []byte {
	key := testVarint((fieldNumber << 3) | 2)
	length := testVarint(uint64(len(value)))
	payload := append(key, length...)
	return append(payload, value...)
}

func testVarint(value uint64) []byte {
	var out []byte
	for value > 0x7f {
		out = append(out, byte(value&0x7f)|0x80)
		value >>= 7
	}
	return append(out, byte(value))
}

func writeTestExtensionZIP(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	files := map[string]string{
		"manifest.json": `{"manifest_version":3,"name":"Open Browser Use","version":"0.1.0","background":{"service_worker":"background.js"},"permissions":["nativeMessaging"]}`,
		"background.js": "chrome.runtime.onInstalled.addListener(() => {});\n",
	}
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			_ = writer.Close()
			return err
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			_ = writer.Close()
			return err
		}
	}
	return writer.Close()
}

func readManifestFromZIP(path string) ([]byte, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name != "manifest.json" {
			continue
		}
		source, err := file.Open()
		if err != nil {
			return nil, err
		}
		payload, readErr := io.ReadAll(source)
		closeErr := source.Close()
		if readErr != nil {
			return nil, readErr
		}
		return payload, closeErr
	}
	return nil, errors.New("manifest.json not found in ZIP")
}
