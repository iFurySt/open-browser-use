/* global chrome */

async function evaluateInTab(browserApi, tabId, expression, awaitPromise) {
  const results = await browserApi.scripting.executeScript({
    target: { tabId },
    func: async (source, shouldAwait) => {
      try {
        let value = globalThis.eval(source);
        if (shouldAwait && value && typeof value.then === "function") {
          value = await value;
        }
        return { ok: true, value };
      } catch (error) {
        return {
          ok: false,
          error: error instanceof Error ? error.message : String(error)
        };
      }
    },
    args: [expression, awaitPromise === true]
  });
  const outcome = results?.[0]?.result;
  if (!outcome?.ok) {
    return {
      exceptionDetails: {
        text: outcome?.error ?? "JavaScript evaluation failed"
      }
    };
  }
  return {
    result: remoteObject(outcome.value)
  };
}

function remoteObject(value) {
  if (value === undefined) {
    return { type: "undefined" };
  }
  if (value === null) {
    return { type: "object", subtype: "null", value: null };
  }
  const type = typeof value;
  return {
    type,
    value,
    ...(type === "object" ? { description: Array.isArray(value) ? "Array" : "Object" } : {})
  };
}

async function executeFirefoxCommand(browserApi, target, method, commandParams) {
  const tabId = target?.tabId;
  switch (method) {
    case "Page.enable":
    case "Runtime.enable":
    case "Network.enable":
      return {};
    case "Page.navigate":
      if (!Number.isInteger(tabId) || typeof commandParams.url !== "string") {
        throw new Error("Page.navigate requires a tab target and URL");
      }
      await browserApi.tabs.update(tabId, { url: commandParams.url });
      return { frameId: String(tabId) };
    case "Page.reload":
      if (!Number.isInteger(tabId)) {
        throw new Error("Page.reload requires a tab target");
      }
      await browserApi.tabs.reload(tabId, { bypassCache: commandParams.ignoreCache === true });
      return {};
    case "Page.close":
      if (!Number.isInteger(tabId)) {
        throw new Error("Page.close requires a tab target");
      }
      await browserApi.tabs.remove(tabId);
      return {};
    case "Runtime.evaluate":
      if (!Number.isInteger(tabId) || typeof commandParams.expression !== "string") {
        throw new Error("Runtime.evaluate requires a tab target and expression");
      }
      return evaluateInTab(
        browserApi,
        tabId,
        commandParams.expression,
        commandParams.awaitPromise
      );
    case "Target.getTargets": {
      const tabs = await browserApi.tabs.query({});
      return {
        targetInfos: tabs.flatMap((tab) =>
          Number.isInteger(tab.id)
            ? [{ targetId: String(tab.id), type: "page", title: tab.title ?? "", url: tab.url ?? "", attached: false }]
            : []
        )
      };
    }
    case "Browser.getVersion": {
      const info = await browserApi.runtime.getBrowserInfo?.();
      return {
        protocolVersion: "firefox-webextension-compat",
        product: info ? `${info.name}/${info.version}` : "Firefox",
        revision: info?.buildID ?? "",
        userAgent: globalThis.navigator?.userAgent ?? "",
        jsVersion: ""
      };
    }
    default:
      throw new Error(
        `CDP method ${String(method)} is not supported by Firefox-based browsers; ` +
          "use Page.navigate, Page.reload, Page.close, Runtime.evaluate, or Target.getTargets"
      );
  }
}

globalThis.openBrowserUseFirefoxExecuteCommand = executeFirefoxCommand;
globalThis.openBrowserUseFirefoxRemoteObject = remoteObject;
