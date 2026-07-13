import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";
import vm from "node:vm";

const source = fs.readFileSync(new URL("./firefox-compat.js", import.meta.url), "utf8");

function loadCompatibility() {
  const context = vm.createContext({ console });
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
