## [2026-09-08 21:00] | Task: Preserve live sockets on dial errors

### Execution Context

- Agent: Codex (GPT-6), Codex desktop

### User Query

> Fix issue #22 and submit a PR, following the repository's test conventions.

### Changes

- Restrict CLI socket cleanup to ENOENT and ECONNREFUSED across registry lookup, profile selection, directory scanning, and background cleanup.
- Preserve other dial errors in discovery failures, including errors from fallback scans.
- Add regression coverage for wrapped syscall errors; update architecture, troubleshooting, and release notes.

### Design Intent

A failed connection does not prove a socket is stale. In particular, a sandbox can deny a connection while still allowing the CLI to unlink the socket. Preserve the host's endpoint on permission errors, timeouts, and other unclassified failures.

### Files Modified

- `cmd/open-browser-use/main.go`
- `cmd/open-browser-use/main_test.go`
- `skills/open-browser-use/references/troubleshooting.md`
- `docs/ARCHITECTURE.md`
- `docs/releases/feature-release-notes.md`

### Validation

- `TMPDIR=/tmp make ci` passed. A short temporary root avoids macOS Unix socket path limits in the existing tests.
- A temporary CLI build preserved a live test socket on sandbox EPERM through profile selection, active registry lookup, and registry-free scanning. Registry lookup also preserved `active.json`.
