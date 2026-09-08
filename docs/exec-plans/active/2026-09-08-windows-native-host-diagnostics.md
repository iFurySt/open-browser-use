# Windows native host discovery diagnostics (#20)

## Goal and scope

Diagnose why Chrome cannot find a host that starts successfully by hand. Add a
read-only Windows diagnostic script, automated fixture coverage, a Windows CI
check, and troubleshooting instructions. Do not change registry entries or
launch/stop Chrome or the host during diagnosis.

## Evidence and constraints

- Issue #20 verifies a per-user registration and a manually launched host, but
  does not check `NativeMessagingUserLevelHosts` or the effective registry lookup.
- Chromium's `launch_context_win.cc` looks in HKCU when user-level hosts are
  allowed, then HKLM; each hive probes the 32-bit view before the 64-bit view.
- The reporting Windows/Chrome environment is not currently available locally.
  Do not claim a confirmed fix or close #20 until that environment is verified.

## Steps and validation

- [x] Inspect issue, installer, and Chromium lookup implementation.
- [x] Implement read-only policy, registration and manifest diagnosis.
- [x] Test missing/invalid manifests, origin mismatch, shadowing, and disabled
      user-level hosts locally; add real registry tests for Windows CI.
- [x] Run `make ci` and local PowerShell fixture tests.
- [ ] Publish a draft PR and verify Windows CI registry integration.
- [ ] Verify the issue reporter's Chrome environment and implement any confirmed
      runtime/installer correction before marking #20 fixed.

## References

- https://github.com/iFurySt/open-browser-use/issues/20
- https://github.com/chromium/chromium/blob/main/chrome/browser/extensions/api/messaging/launch_context_win.cc
- https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging
