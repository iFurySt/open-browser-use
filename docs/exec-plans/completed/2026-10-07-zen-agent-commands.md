# Zen agent commands

Extend the existing CLI/MCP runner using the installed extension's navigation and JavaScript evaluation support. Keep the same session and explicit browser/profile selectors. Do not change native manifests or install an extension.

Observed gaps: direct CLI `page-info`/`wait-load` are missing despite runner support; MCP offers only raw CDP for forms; connected JSON omits unresolved hosts; Zen cannot intercept native file choosers. Agents recreate native setters, verification and upload-byte plumbing in task scripts.

Implement shared DOM inspection, evaluation, clicking, guarded form filling and file-input upload commands. Expose backend capabilities and connected targets in MCP. Bound output and uploads, reject ambiguous/hidden targets, and surface JavaScript errors. Update skill instructions around explicit browser routing, readiness and authorization already supplied by the user.

Validate Go/MCP contracts, DOM behavior including no implicit submission, and an isolated browser fixture in real Zen. Record final checks, commit and push to the existing fork branch.

## Completed

Implemented focused shared host commands and MCP tools, direct legacy page-info/
wait-load commands, backend capability reporting, one-time MCP route selection,
and preservation of unresolved hosts in profiles JSON. Updated the skill and
parameter reference without changing installed extensions or native manifests.

Integrated the fork's newer JSON-RPC notification handling, WebMCP bridge,
Windows manifest separation, and ungrouped session ownership fixes. Resolved
conflicts by preserving incoming fixes and the new host commands/skill guidance.

Final validation: `go test ./...`, `go vet ./...`, 14 extension Node tests,
`pnpm typecheck`, skill quick validation, docs/repo hygiene, and diff checks passed.
A fresh localhost fixture smoke test passed in real Zen after integration; the
local CLI wrapper also returned accurate Zen capabilities with the existing
extension 0.1.41. The temporary fixture tabs were closed and user tabs unchanged.

Limits: main-document CSS helpers, synthetic events, 512 KiB aggregate upload
limit, no cross-origin/shadow-root traversal, and no Zen full CDP/native chooser.
Multi-field writes are prevalidated but not transactional across site handlers.
No extension package or public release was produced; the existing wrapper
provides the updated host/CLI, and MCP must restart to discover added tools.
