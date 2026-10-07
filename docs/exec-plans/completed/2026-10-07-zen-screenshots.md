# Zen screenshot support

Implement a bounded Page.captureScreenshot compatibility adapter using Firefox
tabs.captureTab, preserving session/tab ownership and never switching focus to
capture a different active tab. Support PNG/JPEG, quality, page-relative clips
and scale; expose full-page capture through layout metrics. Reject unsupported
CDP options rather than pretending to implement full CDP.

Expose a screenshot CLI/MCP tool. Return an MCP image or write an explicitly
requested local file without overwriting existing output by default. Report
actual screenshot support from extension capabilities, including older extension
versions that lack it. Update the skill, docs and tests; prepare a Zen package,
verify in the real browser where feasible, record remaining runtime requirements,
and push to the existing fork branch.

## Completed

Implemented the adapter, CSS layout metrics, CLI/MCP screenshot command,
image response blocks, private protected output files and capability discovery.
Updated the skill, parameter reference, README and architecture/release notes.

Validation: Go tests/vet, 17 extension Node tests (including shared ownership
checks), pnpm typecheck, skill validation, script syntax, docs/repo hygiene and
diff checks passed. The Zen XPI packaging script completed.

An isolated real Zen 1.22.2b smoke passed: viewport 1167x752, scaled PNG clip
400x200, JPEG 120x90 and full-page 1167x2400, with pixel checks for the fixture
and its offscreen footer. The disposable profile/browser was closed. A fresh
test-profile permission setup initially did not notify the running extension;
the bootstrap now emits the permission change to that isolated extension.

The user's installed extension 0.1.41 remains unchanged and correctly reports
no screenshot support. Activating the feature requires loading the updated XPI
and restarting MCP to refresh its catalog. No signing or public release was
performed. Limits remain documented: 32 MiB image bytes, 32 megapixels,
16384 pixels per side, PNG/JPEG, and no unsupported CDP options.

Reproduction (substitute local executable paths):

```sh
python3 scripts/zen-screenshot-smoke.py --driver /path/to/geckodriver \
  --zen-binary /path/to/zen-bin --xpi /path/to/updated.xpi \
  --obu /path/to/open-browser-use --artifacts /path/to/test-images
```
