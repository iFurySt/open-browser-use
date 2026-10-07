# Zen agent commands

Extend the existing CLI/MCP runner using the installed extension's navigation and JavaScript evaluation support. Keep the same session and explicit browser/profile selectors. Do not change native manifests or install an extension.

Observed gaps: direct CLI `page-info`/`wait-load` are missing despite runner support; MCP offers only raw CDP for forms; connected JSON omits unresolved hosts; Zen cannot intercept native file choosers. Agents recreate native setters, verification and upload-byte plumbing in task scripts.

Implement shared DOM inspection, evaluation, clicking, guarded form filling and file-input upload commands. Expose backend capabilities and connected targets in MCP. Bound output and uploads, reject ambiguous/hidden targets, and surface JavaScript errors. Update skill instructions around explicit browser routing, readiness and authorization already supplied by the user.

Validate Go/MCP contracts, DOM behavior including no implicit submission, and an isolated browser fixture in real Zen. Record final checks, commit and push to the existing fork branch.
