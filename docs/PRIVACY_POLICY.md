# Open Browser Use Privacy Policy

Last updated: July 13, 2026

Open Browser Use provides Chrome and Zen Browser extensions that connect the
browser to a local Open Browser Use native messaging host. It is designed for
browser automation by compatible local agent tools.

## Data handled by the extension

The extension may access browser tabs, tab URLs, page content, browser history,
download metadata, browser interaction state, and data visible in pages that the
user or a paired local agent chooses to automate. Depending on the pages being
used, this can include personally identifiable information, authentication
information, personal communications, financial information, health information,
location information, web history, user activity, and website content.

On Zen Browser, reading and interacting with page content requires a one-time
optional page interaction permission. The permission applies globally to
ordinary websites and can be revoked from Zen's extension permissions UI.

## How data is used

Data is used only to provide the extension's core browser automation bridge:
inspecting browser state, controlling tabs, executing browser debugging commands,
forwarding browser events, and communicating with the paired local native host.

Open Browser Use does not sell user data. Open Browser Use does not use or
transfer user data for advertising, creditworthiness, lending, or purposes
unrelated to browser automation.

## Data transfer

The extension communicates with the Open Browser Use native messaging host on
the user's device. The extension itself does not send data to an Open Browser
Use cloud service. A local agent runtime or application connected to the native
host may process the browser data it requests; users should only connect trusted
local tools.

## Data storage

The extension may store local operational state in browser extension storage,
such as session metadata needed to recover tab groups and native host status.
This data remains in the user's Chrome profile unless the user removes the
extension or clears browser extension data.

## Contact

For privacy or support questions, use the project issue tracker:

<https://github.com/iFurySt/open-browser-use/issues>
