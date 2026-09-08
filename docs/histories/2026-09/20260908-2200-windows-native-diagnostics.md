## [2026-09-08 22:00] | Task: Diagnose Windows native host discovery

### User Query

> 分别处理现有 Issue，分别提交 PR。

### Changes and intent

- Add a read-only PowerShell diagnostic for issue #20. Inspect host registration
  in Chrome lookup order and check manifest name/type/origin, executable presence,
  and the user-level host policy missing from the original report.
- Report both user-level policy scenarios rather than inferring effective Chrome
  policy from local registry values. Never change registry values or start/stop
  browser or host processes during diagnosis.
- Add fixture tests and a Windows CI job exercising real Unicode registry reads
  under temporary test keys. Document the diagnostic and its limits.

### Files

- `scripts/diagnose-windows-native-host.ps1`
- `scripts/diagnose-windows-native-host.test.ps1`
- `.github/workflows/ci.yml`
- `skills/open-browser-use/references/troubleshooting.md`
- `docs/exec-plans/active/2026-09-08-windows-native-host-diagnostics.md`

### Validation and remaining work

- Full `make ci` passed with Node 24, frozen-lockfile dependencies, and
  `TMPDIR=/tmp`. PowerShell 7.6.5 fixture tests passed locally; the added Windows CI job also passed, including real registry
  integration in both views.
- This is diagnostic progress, not a confirmed resolution of #20. The original
  Windows/Chrome failure still needs reproduction and effective-policy evidence.
