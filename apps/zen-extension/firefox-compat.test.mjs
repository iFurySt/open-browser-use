import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";
import vm from "node:vm";

const source = fs.readFileSync(new URL("./firefox-compat.js", import.meta.url), "utf8");

function loadCompatibility() {
  const context = vm.createContext({ clearTimeout, console, setTimeout });
  vm.runInContext(source, context);
  return context;
}

function local(value) {
  return JSON.parse(JSON.stringify(value));
}

test("emulates navigation and target listing", async () => {
  const context = loadCompatibility();
  const calls = [];
  const browserApi = {
    tabs: {
      update: async (...args) => calls.push(["update", ...args]),
      query: async () => [{ id: 7, title: "Example", url: "https://example.com" }]
    }
  };

  const navigate = await context.openBrowserUseFirefoxExecuteCommand(
    browserApi,
    { tabId: 7 },
    "Page.navigate",
    { url: "https://example.com" }
  );
  assert.deepEqual(local(navigate), { frameId: "7" });
  assert.deepEqual(local(calls), [["update", 7, { url: "https://example.com" }]]);

  const targets = await context.openBrowserUseFirefoxExecuteCommand(
    browserApi,
    {},
    "Target.getTargets",
    {}
  );
  assert.equal(targets.targetInfos[0].targetId, "7");
});

test("returns CDP-shaped values and rejects unsupported methods", async () => {
  const context = loadCompatibility();
  assert.deepEqual(
    local(context.openBrowserUseFirefoxRemoteObject(null)),
    { type: "object", subtype: "null", value: null }
  );
  await assert.rejects(
    context.openBrowserUseFirefoxExecuteCommand({ tabs: {} }, { tabId: 1 }, "DOM.getDocument", {}),
    /not supported by Firefox-based browsers/
  );
});

test("evaluates page expressions through the isolated user script world", async () => {
  const context = loadCompatibility();
  const calls = [];
  const browserApi = {
    userScripts: {
      execute: async (injection) => {
        calls.push(injection);
        return [{ frameId: 0, result: { title: "Example", inputCount: 2 } }];
      }
    }
  };

  const response = await context.openBrowserUseFirefoxExecuteCommand(
    browserApi,
    { tabId: 7 },
    "Runtime.evaluate",
    { expression: "({ title: document.title, inputCount: document.querySelectorAll('input').length })" }
  );

  assert.deepEqual(local(response), {
    result: {
      type: "object",
      value: { title: "Example", inputCount: 2 },
      description: "Object"
    }
  });
  assert.deepEqual(local(calls), [
    {
      target: { tabId: 7 },
      js: [
        {
          code: "({ title: document.title, inputCount: document.querySelectorAll('input').length })\n"
        }
      ],
      world: "USER_SCRIPT",
      worldId: "open-browser-use",
      injectImmediately: true
    }
  ]);
});

test("reports missing permission and user script errors as CDP exceptions", async () => {
  const context = loadCompatibility();
  const missingPermission = await context.openBrowserUseFirefoxExecuteCommand(
    {},
    { tabId: 7 },
    "Runtime.evaluate",
    { expression: "document.title" }
  );
  assert.match(missingPermission.exceptionDetails.text, /Enable page interaction/);

  const scriptError = await context.openBrowserUseFirefoxExecuteCommand(
    { userScripts: { execute: async () => [{ frameId: 0, error: "document is not defined" }] } },
    { tabId: 7 },
    "Runtime.evaluate",
    { expression: "document.title" }
  );
  assert.deepEqual(local(scriptError), {
    exceptionDetails: { text: "document is not defined" }
  });
});

test("supports current Zen through a registered Firefox user-script bridge", async () => {
  const context = loadCompatibility();
  const calls = [];
  let extensionMessageListener;
  let userScriptConnectListener;
  let userScriptMessageListener;
  const browserApi = {
    permissions: {
      contains: async () => false,
      onAdded: { addListener: () => {} }
    },
    runtime: {
      onMessage: {
        addListener: (listener) => {
          extensionMessageListener = listener;
        }
      },
      onUserScriptConnect: {
        addListener: (listener) => {
          userScriptConnectListener = listener;
        }
      }
    },
    userScripts: {
      configureWorld: async (options) => calls.push(["configureWorld", options]),
      getScripts: async () => [],
      register: async (scripts) => calls.push(["register", scripts])
    }
  };

  context.openBrowserUseFirefoxInitializeUserScripts(browserApi);
  const setup = await new Promise((resolve) => {
    const handled = extensionMessageListener(
      { type: "ENABLE_OPEN_BROWSER_USE_PAGE_INTERACTION" },
      {},
      resolve
    );
    assert.equal(handled, true);
  });
  assert.deepEqual(local(setup), { ok: true });
  assert.deepEqual(local(calls), [
    [
      "configureWorld",
      {
        worldId: "open-browser-use",
        csp: "script-src 'self' 'unsafe-eval'; object-src 'none';",
        messaging: true
      }
    ],
    [
      "register",
      [
        {
          id: "open-browser-use-page-bridge",
          matches: ["<all_urls>"],
          js: [{ file: "user-script-bridge.js" }],
          runAt: "document_start",
          world: "USER_SCRIPT",
          worldId: "open-browser-use"
        }
      ]
    ]
  ]);

  const port = {
    name: "open-browser-use-page-bridge",
    sender: { tab: { id: 7 }, userScriptWorldId: "open-browser-use" },
    disconnect: () => assert.fail("valid bridge must not be disconnected"),
    onDisconnect: { addListener: () => {} },
    onMessage: {
      addListener: (listener) => {
        userScriptMessageListener = listener;
      }
    },
    postMessage: (message) => {
      userScriptMessageListener({
        type: "OPEN_BROWSER_USE_EVALUATE_RESULT",
        id: message.id,
        ok: true,
        value: "Mail-Tester"
      });
    }
  };
  userScriptConnectListener(port);

  const response = await context.openBrowserUseFirefoxExecuteCommand(
    browserApi,
    { tabId: 7 },
    "Runtime.evaluate",
    { expression: "document.title" }
  );
  assert.deepEqual(local(response), {
    result: { type: "string", value: "Mail-Tester" }
  });
});
