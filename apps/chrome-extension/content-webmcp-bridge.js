(() => {
  const STATE_KEY = Symbol.for("open-browser-use.webmcp.bridge");
  const REQUEST_SOURCE = "open-browser-use-webmcp-bridge";
  const RESPONSE_SOURCE = "open-browser-use-webmcp-page";
  const PING_MESSAGE = "OPEN_BROWSER_USE_WEBMCP_PING";
  const REQUEST_MESSAGE = "OPEN_BROWSER_USE_WEBMCP_REQUEST";
  const DEFAULT_BRIDGE_TIMEOUT_MS = 5_000;
  const BRIDGE_TIMEOUT_GRACE_MS = 1_000;

  if (globalThis[STATE_KEY]) {
    return;
  }

  const pending = new Map();
  Object.defineProperty(globalThis, STATE_KEY, {
    configurable: false,
    enumerable: false,
    value: { pending },
    writable: false
  });

  window.addEventListener("message", (event) => {
    if (event.source !== window || !isPageResponse(event.data)) {
      return;
    }
    const waiter = pending.get(event.data.requestId);
    if (!waiter) {
      return;
    }
    pending.delete(event.data.requestId);
    clearTimeout(waiter.timeoutId);
    waiter.resolve(
      event.data.ok
        ? { ok: true, result: event.data.result }
        : { ok: false, error: typeof event.data.error === "string" ? event.data.error : "WebMCP request failed" }
    );
  });

  chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
    if (message?.type === PING_MESSAGE) {
      sendResponse({ ok: true });
      return false;
    }
    if (message?.type !== REQUEST_MESSAGE) {
      return false;
    }

    const requestId = createRequestId();
    const timeoutMs = bridgeTimeout(message);
    const responsePromise = new Promise((resolve) => {
      const timeoutId = setTimeout(() => {
        pending.delete(requestId);
        resolve({ ok: false, error: `WebMCP page bridge timed out after ${timeoutMs}ms` });
      }, timeoutMs);
      pending.set(requestId, { resolve, timeoutId });
      window.postMessage(
        {
          ...message,
          type: undefined,
          source: REQUEST_SOURCE,
          requestId
        },
        "*"
      );
    });
    void responsePromise.then(sendResponse);
    return true;
  });

  function bridgeTimeout(message) {
    if (message.action === "invoke" && Number.isInteger(message.timeout_ms) && message.timeout_ms > 0) {
      return message.timeout_ms + BRIDGE_TIMEOUT_GRACE_MS;
    }
    return DEFAULT_BRIDGE_TIMEOUT_MS;
  }

  function createRequestId() {
    if (typeof crypto?.randomUUID === "function") {
      return crypto.randomUUID();
    }
    return `obu-webmcp-request-${Date.now()}-${Math.random()}`;
  }

  function isPageResponse(value) {
    return (
      value &&
      typeof value === "object" &&
      value.source === RESPONSE_SOURCE &&
      typeof value.requestId === "string" &&
      typeof value.ok === "boolean"
    );
  }
})();
