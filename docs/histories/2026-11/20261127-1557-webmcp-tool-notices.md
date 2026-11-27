## [2026-11-27 15:57] | Task: Surface WebMCP tool changes

### Execution context

- Agent ID: Codex
- Base model: GPT-6
- Runtime: Codex desktop on macOS

### User request

> Make page tool discovery visible to Chrome agents and submit the change from a fork.

### Changes

The CLI requests a bounded tool preview on claim, active tab lookup and tab CDP commands. The extension reports the first available tools and changes to registrations. It checks session ownership before and after the read. The native relay drops late replies for closed request connections.

### Design intent

A one-shot CLI process can miss events between commands. Deliver the notice alongside the next eligible response. Keep explicit tool listing responsible for issuing invocation handles.

### Verification

The Chrome 156 smoke test used a temporary profile, the unpacked extension, a native host and a local fixture. Claim reported an existing tool. A passive check preserved an invocation handle. Later commands reported a new tool and removal, and repeated reads stayed quiet.

### Files

- Chrome service worker, WebMCP page bridge and extension tests
- Go CLI, native relay and tests
- TypeScript protocol, agent skill, execution plan and release notes
