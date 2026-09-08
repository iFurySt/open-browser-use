# Open Browser Use Installation

Read this reference when the user asks to install, verify, repair, or explain Open Browser Use setup.

## Components

- Browser extension: the Chrome or Zen-side controller. Installing or enabling it may require user approval.
- Native host and CLI: the local `open-browser-use` binary, also exposed as `obu` when installed from supported packages.
- SDKs: JavaScript, Python, and Go clients that connect to the active native host socket.

## Install The CLI

Use one of the supported package routes:

```sh
npm install -g open-browser-use
```

```sh
brew install iFurySt/open-browser-use/open-browser-use
```

Verify:

```sh
open-browser-use version
obu version
```

If the short alias is unavailable, use `open-browser-use`.
Running `open-browser-use` with no subcommand prints the CLI version, browser
extension detection status, extension version when available, and the next setup
or upgrade command.

## Set Up A Browser

After installing the CLI, register the native messaging host and open the Chrome Web Store page for the matching extension:

```sh
open-browser-use setup
```

Ask the user to install or enable Open Browser Use from the opened store page. Chrome may ask the user to confirm, enable the extension, or restart. Do not bypass this user step.

For Chrome Beta, register that browser explicitly:

```sh
open-browser-use setup --browser chrome-beta
```

For Zen Browser, download the matching
`open-browser-use-zen-extension-<version>.xpi` from GitHub Releases. Open
`about:debugging#/runtime/this-firefox`, choose **Load Temporary Add-on**, and
select the XPI. This is an experimental, unsigned temporary installation: Zen
removes the add-on at restart, so load it again before using the CLI. Register
Firefox native messaging:

```sh
open-browser-use install-manifest --browser zen
open-browser-use profiles --connected
```

Open the extension popup and enable Page Interaction (`userScripts`); the
current ordinary tab reloads once. Check permission again after reloading the
temporary add-on. On Windows the Firefox manifest uses a separate
`NativeMessagingHosts/firefox/` directory; Chrome keeps its existing path. If an
earlier experimental installation overwrote Chrome registration, rerun both
`install-manifest --browser chrome` and `install-manifest --browser zen` with
the fixed CLI.

The Zen route supports the normal tab/history/session commands plus the core
`Page.navigate`, `Page.reload`, `Page.close`, `Runtime.evaluate`, and
`Target.getTargets` compatibility calls. Firefox does not implement Chrome's
extension debugger API, so arbitrary CDP methods and local file chooser path
injection are unavailable.

For BitBrowser, install or load the extension in the target BitBrowser instance,
then register the native host manifest into that instance's user-data directory:

```sh
open-browser-use install-manifest --browser <bitbrowser-instance-id>
```

Use `open-browser-use profiles --connected --json` to see BitBrowser instance
ids in the `browserInstance` field once the instance is detectable.

While the Chrome Web Store item is unavailable or pending review, use the release ZIP path:

```sh
open-browser-use setup beta
```

This downloads the latest keyed `open-browser-use-chrome-extension-*.zip` from GitHub Releases and registers the native host for that stable extension id. It opens `chrome://extensions/` and reveals the ZIP in Finder or the system file manager only when the browser extension is missing or older than the CLI-expected version. Ask the user to enable Developer mode and drag that ZIP into the Chrome extensions page when setup prints that next step.

For Chrome Beta, use `open-browser-use setup beta --browser chrome-beta`.

Repair only the native host manifest:

```sh
open-browser-use install-manifest
```

Use `--browser chrome-beta`, `--browser zen`, or `--browser <bitbrowser-instance-id>` to repair a
non-default browser.

Print the manifest without installing:

```sh
open-browser-use manifest
```

## Platform Notes

- macOS and Windows can require the user to approve or enable the extension after Chrome sees it.
- Linux external extension registration can require elevated permissions depending on Chrome installation paths.
- The browser native messaging host name is `com.ifuryst.open_browser_use.extension`.
- The default socket registry is under `/tmp/open-browser-use/` on Unix-like systems.

## Verification

Run:

```sh
open-browser-use ping --session-id "$OBU_SESSION_ID"
open-browser-use info --session-id "$OBU_SESSION_ID"
open-browser-use user-tabs --session-id "$OBU_SESSION_ID"
```

For one-off installation checks, a temporary session id is enough. Agent browser
tasks should still create and reuse a task-unique session id before opening or
claiming tabs.

If `ping` cannot communicate with Chrome, ask the user whether Chrome is installed and running, whether the extension is enabled, and whether they approved any Chrome prompt. Then use [troubleshooting.md](troubleshooting.md).
