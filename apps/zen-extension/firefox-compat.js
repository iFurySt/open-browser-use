/* global chrome */

const FIREFOX_USER_SCRIPT_ID = "open-browser-use-page-bridge";
const FIREFOX_USER_SCRIPT_WORLD = "open-browser-use";
const FIREFOX_USER_SCRIPT_PORT = "open-browser-use-page-bridge";
const FIREFOX_EVALUATION_TIMEOUT_MS = 15000;
const firefoxUserScriptPorts = new Map();
const firefoxPendingEvaluations = new Map();
let firefoxEvaluationId = 0;
let firefoxUserScriptListenersRegistered = false;
let firefoxUserScriptSetupPromise;

async function evaluateInTab(browserApi, tabId, expression) {
  if (browserApi.userScripts?.execute) {
    const results = await browserApi.userScripts.execute({
      target: { tabId },
      js: [{ code: `${expression}\n` }],
      world: "USER_SCRIPT",
      worldId: FIREFOX_USER_SCRIPT_WORLD,
      injectImmediately: true
    });
    const outcome = results?.[0]?.result;
    const error = results?.[0]?.error;
    if (error != null) {
      return {
        exceptionDetails: {
          text: typeof error === "string" ? error : String(error)
        }
      };
    }
    return {
      result: remoteObject(outcome)
    };
  }

  const port = firefoxUserScriptPorts.get(tabId);
  if (!port) {
    return {
      exceptionDetails: {
        text:
          "Page interaction is not ready for this tab. Enable page interaction in the " +
          "Open Browser Use popup, then reload this tab once."
      }
    };
  }

  const id = ++firefoxEvaluationId;
  return new Promise((resolve) => {
    const timeout = setTimeout(() => {
      firefoxPendingEvaluations.delete(id);
      resolve({ exceptionDetails: { text: "JavaScript evaluation timed out" } });
    }, FIREFOX_EVALUATION_TIMEOUT_MS);
    firefoxPendingEvaluations.set(id, { resolve, timeout, tabId });
    try {
      port.postMessage({ type: "OPEN_BROWSER_USE_EVALUATE", id, expression });
    } catch (error) {
      clearTimeout(timeout);
      firefoxPendingEvaluations.delete(id);
      resolve({
        exceptionDetails: { text: error instanceof Error ? error.message : String(error) }
      });
    }
  });
}

function handleFirefoxUserScriptResult(message, tabId) {
  if (message?.type !== "OPEN_BROWSER_USE_EVALUATE_RESULT" || !Number.isInteger(message.id)) {
    return;
  }
  const pending = firefoxPendingEvaluations.get(message.id);
  if (!pending || pending.tabId !== tabId) {
    return;
  }
  clearTimeout(pending.timeout);
  firefoxPendingEvaluations.delete(message.id);
  pending.resolve(
    message.ok === true
      ? { result: remoteObject(message.value) }
      : { exceptionDetails: { text: message.error ?? "JavaScript evaluation failed" } }
  );
}

function registerFirefoxUserScriptListeners(browserApi) {
  if (firefoxUserScriptListenersRegistered || !browserApi.runtime?.onUserScriptConnect) {
    return;
  }
  firefoxUserScriptListenersRegistered = true;
  browserApi.runtime.onUserScriptConnect.addListener((port) => {
    const tabId = port.sender?.tab?.id;
    if (
      port.name !== FIREFOX_USER_SCRIPT_PORT ||
      port.sender?.userScriptWorldId !== FIREFOX_USER_SCRIPT_WORLD ||
      !Number.isInteger(tabId)
    ) {
      port.disconnect();
      return;
    }
    firefoxUserScriptPorts.set(tabId, port);
    port.onMessage.addListener((message) => handleFirefoxUserScriptResult(message, tabId));
    port.onDisconnect.addListener(() => {
      if (firefoxUserScriptPorts.get(tabId) === port) {
        firefoxUserScriptPorts.delete(tabId);
      }
    });
  });
}

async function ensureFirefoxUserScriptBridge(browserApi) {
  if (!browserApi.userScripts?.register || !browserApi.userScripts?.configureWorld) {
    throw new Error(
      "This Zen version does not support Firefox MV3 user scripts (Firefox 136+ required)."
    );
  }
  if (firefoxUserScriptSetupPromise) {
    return firefoxUserScriptSetupPromise;
  }
  firefoxUserScriptSetupPromise = (async () => {
    registerFirefoxUserScriptListeners(browserApi);
    await browserApi.userScripts.configureWorld({
      worldId: FIREFOX_USER_SCRIPT_WORLD,
      csp: "script-src 'self' 'unsafe-eval'; object-src 'none';",
      messaging: true
    });
    const scripts = await browserApi.userScripts.getScripts({ ids: [FIREFOX_USER_SCRIPT_ID] });
    if (scripts.length === 0) {
      await browserApi.userScripts.register([
        {
          id: FIREFOX_USER_SCRIPT_ID,
          matches: ["<all_urls>"],
          js: [{ file: "user-script-bridge.js" }],
          runAt: "document_start",
          world: "USER_SCRIPT",
          worldId: FIREFOX_USER_SCRIPT_WORLD
        }
      ]);
    }
  })();
  try {
    await firefoxUserScriptSetupPromise;
  } catch (error) {
    firefoxUserScriptSetupPromise = undefined;
    throw error;
  }
}

function initializeFirefoxUserScripts(browserApi) {
  browserApi.permissions?.onAdded?.addListener((permissions) => {
    if (permissions.permissions?.includes("userScripts")) {
      void ensureFirefoxUserScriptBridge(browserApi).catch(() => {});
    }
  });
  browserApi.permissions?.onRemoved?.addListener((permissions) => {
    if (permissions.permissions?.includes("userScripts")) {
      firefoxUserScriptSetupPromise = undefined;
      firefoxUserScriptPorts.clear();
    }
  });
  browserApi.runtime?.onMessage?.addListener((message, _sender, sendResponse) => {
    if (message?.type !== "ENABLE_OPEN_BROWSER_USE_PAGE_INTERACTION") {
      return false;
    }
    void ensureFirefoxUserScriptBridge(browserApi).then(
      () => sendResponse({ ok: true }),
      (error) => sendResponse({
        ok: false,
        error: error instanceof Error ? error.message : String(error)
      })
    );
    return true;
  });
  void browserApi.permissions?.contains?.({ permissions: ["userScripts"] }).then((granted) => {
    if (granted) {
      void ensureFirefoxUserScriptBridge(browserApi).catch(() => {});
    }
  });
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
        commandParams.expression
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
globalThis.openBrowserUseFirefoxInitializeUserScripts = initializeFirefoxUserScripts;

if (globalThis.chrome) {
  initializeFirefoxUserScripts(chrome);
}
