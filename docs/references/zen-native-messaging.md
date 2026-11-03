# Zen native messaging source verification

Checked on 2026-09-08 for PR #14. This is source verification, not a real browser
or Windows registry smoke test.

## Pinned sources

- [Zen desktop 412731f surfer.json](https://github.com/zen-browser/desktop/blob/412731f37e567223097101d9fae9f9d364708b6b/surfer.json)
  selects Firefox 155.0.1.
- [Zen directory-provider patch](https://github.com/zen-browser/desktop/blob/412731f37e567223097101d9fae9f9d364708b6b/src/toolkit/xre/nsXREDirProvider-cpp.patch)
  changes profile directory behavior, not the native manifest lookup branches.
- [Zen runtime patch](https://github.com/zen-browser/desktop/blob/412731f37e567223097101d9fae9f9d364708b6b/src/toolkit/components/extensions/parent/ext-runtime-js.patch)
  changes browser info reporting. Inspection of this revision's extension patch
  tree found no override of NativeManifests or NativeMessaging.
- [Firefox 155.0.1 NativeManifests](https://github.com/mozilla-firefox/firefox/blob/FIREFOX_155_0_1_RELEASE/toolkit/components/extensions/NativeManifests.sys.mjs)
  reads the default value under `Software\Mozilla\NativeMessagingHosts\<name>`
  on Windows, then parses the referenced file and checks `allowed_extensions`.
- [Firefox 155.0.1 NativeMessaging](https://github.com/mozilla-firefox/firefox/blob/FIREFOX_155_0_1_RELEASE/toolkit/components/extensions/NativeMessaging.sys.mjs)
  launches the executable with the manifest path followed by the extension ID.
- [Firefox directory provider](https://github.com/mozilla-firefox/firefox/blob/FIREFOX_155_0_1_RELEASE/toolkit/xre/nsXREDirProvider.cpp)
  keeps user native manifests under macOS Application Support/Mozilla and Linux
  `.mozilla`; these are independent of Zen's profile roots.
- [Mozilla temporary installation guide](https://extensionworkshop.com/documentation/develop/temporary-installation-in-firefox/)
  documents removal of temporary add-ons when the browser restarts.

## Consequences for this repository

The source comparison supports retaining the Mozilla registry vendor and native
lookup locations for Zen. Windows registry values may reference different files;
keep Chrome's existing manifest file and put Firefox's incompatible allowlist
in a `firefox` subdirectory. Keep the basename unchanged so Firefox's launch
arguments still enter host mode. After a previous experimental install overwrote
Chrome's manifest, rerun registration for Chrome and Zen with the fixed CLI.

Tests cover both registration-file write orders and reinstallation, plus the
Windows path branch on a Windows runner. Explicit output paths prevent tests
from changing real registry entries. Source verification does not establish
Flatpak portal availability, extension signing, or real browser connectivity.
