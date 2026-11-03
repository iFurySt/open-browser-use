# Update the Zen fork to upstream 1.1.0

## Goal

Merge upstream `b136cac` into the Zen branch at `98e8317`. Keep Zen DOM commands,
screenshots, native manifests, and session ownership. Install matching local
CLI and extension builds, then push the checked fork branches.

## Decisions

- Keep the existing fork history. Use a merge and a local backup branch.
- Keep upstream Electron IAB and Chromium ZIP setup. Reject Chromium ZIP setup
  for Zen before it reads or changes an extension package.
- Keep both screenshot response paths in MCP. Use upstream object-shaped
  structured content for ordinary responses.
- Set the Zen extension version to 1.1.0 to match the shared runtime and CLI.
- Preserve the current desktop session, extension permissions, and browser tabs.

## Checks

- [x] Fetch both remotes and inspect upstream changes.
- [x] Combine merge conflicts and align the Zen extension version.
- [x] Test the Zen setup rejection before and after the fix.
- [x] Run repository CI and type checks; rebuild the CLI and Zen XPI.
- [x] Update the installed Zen add-on and check live DOM and screenshot support.
- [x] Record results and prepare both fork branches for a normal atomic push.

## Results

Repository CI, type checks, and Go vet passed. The focused setup test failed for
both ZIP entry points before the guard and passed after it. The live DOM test
passed. A fresh MCP server reports version 1.1.0 and exposes the DOM and image
tools. All 13 installed XPI files match the checked package.

The existing live screenshot test assumes 100% page zoom. In the current profile,
its 400 by 200 assertion received 480 by 240 at 120% zoom. Firefox multiplies
capture scale by page zoom in `TabBase.capture`; no capture code change is needed.
A direct live check passed for PNG, JPEG, doubled scale, and the full-page footer
at the current zoom. The exact-dimension smoke passed in a disposable Zen profile:
400 by 200 PNG, 120 by 90 JPEG, and a 1172 by 2400 full page.

The fork keeps upstream `main` as a mirror and the Zen changes on
`agent/zen-browser-support`. Deliver both refs together without a force push.

## Recovery

The branch `backup/zen-before-update-20261028` retains the previous fork head.
The installed extension remains usable until the new package is accepted.
Do not restart the browser or replace its profile files to install the update.
