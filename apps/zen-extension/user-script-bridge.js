/* global browser */

const port = browser.runtime.connect({ name: "open-browser-use-page-bridge" });

port.onMessage.addListener(async (message) => {
  if (
    message?.type !== "OPEN_BROWSER_USE_EVALUATE" ||
    !Number.isInteger(message.id) ||
    typeof message.expression !== "string"
  ) {
    return;
  }
  try {
    let value = globalThis.eval(message.expression);
    if (value && typeof value.then === "function") {
      value = await value;
    }
    port.postMessage({
      type: "OPEN_BROWSER_USE_EVALUATE_RESULT",
      id: message.id,
      ok: true,
      value
    });
  } catch (error) {
    port.postMessage({
      type: "OPEN_BROWSER_USE_EVALUATE_RESULT",
      id: message.id,
      ok: false,
      error: error instanceof Error ? error.message : String(error)
    });
  }
});
