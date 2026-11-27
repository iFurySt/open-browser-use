# Surface WebMCP changes on owned Chrome tabs

## Goal

An agent can discover page tools through the Chrome commands it already uses, including tools registered after page load. See [issue #35](https://github.com/iFurySt/open-browser-use/issues/35).

## Design

1. Read a bounded name preview from the current document. Keep explicit list and invoke behavior intact.
2. Request the preview after a successful claim, a tab CDP command, or an active session tab lookup. Return changes on the requesting connection. Keep existing result values compatible.
3. Check the live extension in an isolated browser, exercise late registration and removal, and run the repository suite.

## Risks and checks

Page registrations and ownership can change during a read. Check tab ownership, the WebMCP setting and the read deadline before delivery. A disconnected client's reply must stay private. Tests cover notice routing, unchanged state, late registration, removal and invocation after a passive read.
