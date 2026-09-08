# Open Browser Use Troubleshooting

Read this reference when setup, connection, browser control, file upload, download, or socket discovery fails.

## First Checks

Start with:

```sh
open-browser-use ping --session-id "$OBU_SESSION_ID"
open-browser-use info --session-id "$OBU_SESSION_ID"
open-browser-use user-tabs --session-id "$OBU_SESSION_ID"
```

For connection checks, set `OBU_SESSION_ID` to a temporary unique value first.
Do not reuse the CLI fallback session for agent browser work.

If these fail:

1. Confirm Chrome is installed.
2. Confirm Chrome is running.
3. Confirm the Open Browser Use extension is installed and enabled.
4. Confirm the native host manifest is installed with `open-browser-use install-manifest` or rerun `open-browser-use setup`.
5. Ask the user to approve any Chrome extension prompt.

Do not silently install, enable, or repair browser integration when the action needs user approval.

## Stale Socket Or Missing Active Host

The CLI first discovers the active socket from the registry. If the registry is
missing, recent CLI versions scan `--socket-dir` for `*.sock` files and connect
to the newest usable socket, then repair the registry. If the registry points to
a stale socket, the CLI removes the stale entry and stale socket file, then tries
the same socket-dir scan. Cleanup only applies to missing sockets or refused
connections. Permission errors and timeouts preserve the socket; discovery
reports those errors if no usable socket is found. If a sandbox blocks local
socket access, retry from an execution context with permission to connect.

Useful flags:

```sh
open-browser-use ping --socket /tmp/open-browser-use/example.sock
open-browser-use ping --socket-dir /tmp/open-browser-use
open-browser-use ping --timeout 20s
```

If no active host exists, opening Chrome with the extension enabled can allow Chrome to start the native host.

## Extension Or Native Host Mismatch

The native host manifest must allow the installed extension id. The default Web Store id is built into the CLI, while `setup beta` uses the keyed GitHub Release ZIP, registers that stable id, and reveals that same ZIP for manual installation.

Use:

```sh
open-browser-use manifest
open-browser-use install-manifest
open-browser-use setup
open-browser-use setup beta
```

If the user installed a custom extension build, pass the extension id explicitly:

```sh
open-browser-use install-manifest --extension-id <chrome-extension-id>
open-browser-use setup --extension-id <chrome-extension-id>
```

## File Upload Issues

Use the Open Browser Use file chooser flow rather than native OS picker automation where possible.

If Chrome blocks local file access for the extension, ask the user to open `chrome://extensions`, open Open Browser Use extension details, and enable file URL access if the task requires local file URLs.

## Permission And Safety Issues

- History, debugger, downloads, tab groups, and broad host access are high-privilege browser capabilities.
- Clipboard reads/writes should happen only for the user-requested task.
- If the user is on a login, payment, approval, CAPTCHA, or destructive workflow, pause and ask before continuing.

## When To Escalate To The User

Ask the user for help when:

- Chrome is not installed.
- Chrome is closed and opening it would interrupt their session.
- Chrome requires extension confirmation or enablement.
- The page requires login, CAPTCHA, hardware key, payment confirmation, or another human-only step.
- The requested browser action affects external systems.
