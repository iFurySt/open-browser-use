// Node test for finalizeTabs debugger detach (the "调试横幅 stays up" fix).
// Run: node apps/chrome-extension/finalize-detach.test.mjs
//
// Loads background.js into a sandbox with stubbed chrome.* APIs, then exercises
// BrowserBackend.finalizeTabs against a session that has the debugger attached
// to its tabs. Verifies that finalize detaches EVERY kept disposition —
// including handoff tabs that stay open — so Chrome's "started debugging this
// browser" banner clears once the turn ends. A handoff tab must stay open and
// grouped; only its debugger attachment is dropped.

import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import vm from "node:vm";
import assert from "node:assert/strict";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const BG_PATH = path.join(HERE, "background.js");

function createChromeFake() {
  const state = {
    nextTabId: 1000,
    nextGroupId: 100,
    tabs: new Map(), // id -> { id, windowId, groupId }
    groups: new Map(), // id -> { id, windowId, title }
    attached: new Set(), // tabIds currently attached via chrome.debugger
    storage: {}
  };

  function disposeEmptyGroups() {
    for (const [gid] of state.groups) {
      const inUse = [...state.tabs.values()].some((t) => t.groupId === gid);
      if (!inUse) state.groups.delete(gid);
    }
  }

  const chrome = {
    storage: {
      local: {
        async get(key) {
          if (typeof key === "string") {
            return key in state.storage ? { [key]: state.storage[key] } : {};
          }
          return { ...state.storage };
        },
        async set(obj) {
          Object.assign(state.storage, obj);
        }
      }
    },
    tabs: {
      async get(tabId) {
        const tab = state.tabs.get(tabId);
        if (!tab) throw new Error(`No tab ${tabId}`);
        return { ...tab };
      },
      async query(filter) {
        const out = [];
        for (const tab of state.tabs.values()) {
          if (filter && typeof filter.groupId === "number" && tab.groupId !== filter.groupId) continue;
          out.push({ ...tab });
        }
        return out;
      },
      async remove(tabIds) {
        const ids = Array.isArray(tabIds) ? tabIds : [tabIds];
        for (const id of ids) {
          state.tabs.delete(id);
          // Chrome auto-detaches the debugger when a tab closes.
          state.attached.delete(id);
        }
        disposeEmptyGroups();
      },
      async group({ groupId, tabIds }) {
        const ids = Array.isArray(tabIds) ? tabIds : [tabIds];
        let targetGroupId = groupId;
        if (typeof targetGroupId !== "number") {
          targetGroupId = state.nextGroupId++;
          const first = state.tabs.get(ids[0]);
          state.groups.set(targetGroupId, { id: targetGroupId, windowId: first?.windowId ?? 1, title: "" });
        }
        for (const id of ids) {
          const t = state.tabs.get(id);
          if (t) t.groupId = targetGroupId;
        }
        return targetGroupId;
      },
      async ungroup(tabIds) {
        const ids = Array.isArray(tabIds) ? tabIds : [tabIds];
        for (const id of ids) {
          const t = state.tabs.get(id);
          if (t) t.groupId = -1;
        }
        disposeEmptyGroups();
      }
    },
    tabGroups: {
      async get(groupId) {
        const g = state.groups.get(groupId);
        if (!g) throw new Error(`No group ${groupId}`);
        return { ...g };
      },
      async query() {
        return [...state.groups.values()].map((g) => ({ ...g }));
      },
      async update(groupId, patch) {
        const g = state.groups.get(groupId);
        if (g) Object.assign(g, patch);
        return g ? { ...g } : {};
      },
      onRemoved: { addListener() {} }
    },
    debugger: {
      async attach({ tabId }) {
        state.attached.add(tabId);
      },
      async detach({ tabId }) {
        if (!state.attached.has(tabId)) {
          throw new Error(`Debugger is not attached to tab ${tabId}`);
        }
        state.attached.delete(tabId);
      },
      onEvent: { addListener() {} },
      onDetach: { addListener() {} }
    },
    runtime: {
      onInstalled: { addListener() {} },
      onStartup: { addListener() {} },
      onSuspend: { addListener() {} },
      onConnectExternal: { addListener() {} },
      onMessage: { addListener() {} }
    },
    alarms: { create() { return Promise.resolve(); }, clear() { return Promise.resolve(); }, onAlarm: { addListener() {} } },
    windows: { onRemoved: { addListener() {} } },
    downloads: { onCreated: { addListener() {} }, onChanged: { addListener() {} }, onDeterminingFilename: { addListener() {} } },
    notifications: { create() {}, onClicked: { addListener() {} } }
  };

  function createTab({ windowId = 1, groupId = -1 } = {}) {
    const id = state.nextTabId++;
    state.tabs.set(id, { id, windowId, groupId });
    return id;
  }
  function createGroup({ windowId = 1, title = "Task - OBU" } = {}) {
    const id = state.nextGroupId++;
    state.groups.set(id, { id, windowId, title });
    return id;
  }

  return { chrome, state, helpers: { createTab, createGroup } };
}

async function loadBackground(bgSource, chromeFake) {
  const inject = `globalThis.__obu_export = (name, value) => { globalThis[name] = value; };`;
  const exporter = `__obu_export("BrowserBackend", BrowserBackend);`;
  const context = vm.createContext({
    chrome: chromeFake,
    console,
    setTimeout,
    clearTimeout,
    setInterval,
    clearInterval,
    queueMicrotask,
    URL,
    TextEncoder,
    TextDecoder,
    structuredClone,
    crypto: globalThis.crypto,
    Date
  });
  vm.runInContext(inject, context);
  vm.runInContext(
    "chrome.runtime.connectNative = () => ({ onMessage: { addListener(){} }, onDisconnect: { addListener(){} }, postMessage(){}, disconnect(){} });",
    context
  );
  vm.runInContext(bgSource + "\n" + exporter, context);
  return { BrowserBackend: context.BrowserBackend };
}

// Wire a session group directly into the backend's store so finalizeTabs sees
// it as an active session with the given tab origins and attached debuggers.
async function seedSession(backend, chrome, { sessionId, groupId, tabOrigins, activeTabId, attached }) {
  const sessionState = await backend.store.getSession(sessionId);
  sessionState.chromeGroupId = groupId;
  sessionState.tabOrigins = { ...tabOrigins };
  sessionState.activeTabId = activeTabId;
  await backend.store.save();
  for (const tabId of attached) {
    backend.attachedTabs.add(tabId);
    chrome.debugger.attach({ tabId }); // mark attached in the fake too
  }
}

async function run() {
  const bgSource = await readFile(BG_PATH, "utf8");

  // === Test 1: handoff tab stays open + grouped, but debugger detaches ===
  {
    const { chrome, state, helpers } = createChromeFake();
    const { BrowserBackend } = await loadBackground(bgSource, chrome);
    const backend = new BrowserBackend();
    await backend.store.ready;

    const groupId = helpers.createGroup({ windowId: 1, title: "Top Nav Position - OBU" });
    const agentTab = helpers.createTab({ windowId: 1, groupId });
    const handoffTab = helpers.createTab({ windowId: 1, groupId });

    await seedSession(backend, chrome, {
      sessionId: "sess-handoff",
      groupId,
      tabOrigins: { [agentTab]: "agent", [handoffTab]: "agent" },
      activeTabId: handoffTab,
      attached: [agentTab, handoffTab]
    });

    await backend.finalizeTabs({
      session_id: "sess-handoff",
      turn_id: "turn-1",
      keep: [{ tabId: handoffTab, status: "handoff" }]
    });

    assert.equal(state.attached.has(handoffTab), false, "handoff tab debugger must be detached (banner would otherwise stay up)");
    assert.equal(state.attached.size, 0, "no tabs should remain attached after finalize");
    assert.ok(state.tabs.has(handoffTab), "handoff tab must stay open");
    assert.equal(state.tabs.get(handoffTab).groupId, groupId, "handoff tab must stay in its task group");
    assert.equal(state.tabs.has(agentTab), false, "unkept agent tab must be closed");
    assert.ok(backend.store.state.sessions["sess-handoff"], "session persists while a handoff tab is kept");
    console.log("test 1 ok: handoff tab detached but kept open and grouped");
  }

  // === Test 2: keep=[] closes agent tab, detaches, and ends the session ===
  {
    const { chrome, state, helpers } = createChromeFake();
    const { BrowserBackend } = await loadBackground(bgSource, chrome);
    const backend = new BrowserBackend();
    await backend.store.ready;

    const groupId = helpers.createGroup({ windowId: 1, title: "SDK template verify - OBU" });
    const agentTab = helpers.createTab({ windowId: 1, groupId });

    await seedSession(backend, chrome, {
      sessionId: "sess-close",
      groupId,
      tabOrigins: { [agentTab]: "agent" },
      activeTabId: agentTab,
      attached: [agentTab]
    });

    await backend.finalizeTabs({
      session_id: "sess-close",
      turn_id: "turn-1",
      keep: []
    });

    assert.equal(state.attached.size, 0, "no tabs should remain attached after finalize");
    assert.equal(state.tabs.has(agentTab), false, "agent tab must be closed");
    assert.equal(state.groups.has(groupId), false, "empty task group must be gone");
    assert.equal(backend.store.state.sessions["sess-close"], undefined, "session must be removed when nothing is kept");
    console.log("test 2 ok: keep=[] closes tab, detaches, ends session");
  }

  console.log("\nAll finalizeTabs detach tests passed.");
}

run().catch((err) => {
  console.error(err);
  process.exit(1);
});
