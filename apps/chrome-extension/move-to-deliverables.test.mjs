// Node test for moveToDeliverables (the deliverable tab-group merge fix).
// Run: node apps/chrome-extension/move-to-deliverables.test.mjs
//
// Loads background.js into a sandbox with stubbed chrome.* APIs, then exercises
// the BrowserBackend.moveToDeliverables method against simulated Chrome
// tab/group state. Verifies that pre-existing deliverable groups (the bug
// scenario from session restore) are reused and de-duplicated instead of
// producing parallel groups with the same title.

import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import vm from "node:vm";
import assert from "node:assert/strict";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const BG_PATH = path.join(HERE, "background.js");
const DELIVERABLE_TITLE = "✅ Open Browser Use";

function createChromeFake() {
  const state = {
    nextTabId: 1000,
    nextGroupId: 100,
    tabs: new Map(), // id -> { id, windowId, groupId }
    groups: new Map(), // id -> { id, windowId, title, color, collapsed }
    storage: {}
  };

  function ensureGroup(groupId, windowId, title = "") {
    if (!state.groups.has(groupId)) {
      state.groups.set(groupId, { id: groupId, windowId, title, color: "grey", collapsed: false });
    }
    return state.groups.get(groupId);
  }

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
      onRemoved: { addListener() {} },
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
      async group({ groupId, tabIds }) {
        if (!Array.isArray(tabIds) || tabIds.length === 0) {
          throw new Error("group() requires tabIds");
        }
        const tabs = tabIds.map((id) => {
          const t = state.tabs.get(id);
          if (!t) throw new Error(`Unknown tab ${id}`);
          return t;
        });
        const windowIds = new Set(tabs.map((t) => t.windowId));
        if (windowIds.size > 1) {
          throw new Error("group() tabs span multiple windows");
        }
        const [windowId] = windowIds;
        let targetGroupId = groupId;
        if (typeof targetGroupId === "number") {
          const existing = state.groups.get(targetGroupId);
          if (!existing) throw new Error(`Unknown group ${targetGroupId}`);
          if (existing.windowId !== windowId) {
            throw new Error("group() target group lives in a different window");
          }
        } else {
          targetGroupId = state.nextGroupId++;
          ensureGroup(targetGroupId, windowId);
        }
        for (const t of tabs) {
          t.groupId = targetGroupId;
        }
        disposeEmptyGroups();
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
      async query(filter = {}) {
        const out = [];
        for (const g of state.groups.values()) {
          if (typeof filter.windowId === "number" && g.windowId !== filter.windowId) continue;
          if (typeof filter.title === "string" && g.title !== filter.title) continue;
          out.push({ ...g });
        }
        return out;
      },
      async update(groupId, patch) {
        const g = state.groups.get(groupId);
        if (!g) throw new Error(`No group ${groupId}`);
        Object.assign(g, patch);
        return { ...g };
      },
      onRemoved: { addListener() {} }
    },
    runtime: { onInstalled: { addListener() {} }, onStartup: { addListener() {} }, onSuspend: { addListener() {} }, onConnectExternal: { addListener() {} }, onMessage: { addListener() {} } },
    alarms: { create() { return Promise.resolve(); }, clear() { return Promise.resolve(); }, onAlarm: { addListener() {} } },
    debugger: { onEvent: { addListener() {} }, onDetach: { addListener() {} } },
    windows: { onRemoved: { addListener() {} } },
    downloads: { onCreated: { addListener() {} }, onChanged: { addListener() {} }, onDeterminingFilename: { addListener() {} } },
    webNavigation: { onCommitted: { addListener() {} }, onCompleted: { addListener() {} }, onDOMContentLoaded: { addListener() {} } },
    notifications: { create() {}, onClicked: { addListener() {} } }
  };

  // helpers used by tests
  function createTab({ windowId = 1, groupId = -1 } = {}) {
    const id = state.nextTabId++;
    state.tabs.set(id, { id, windowId, groupId });
    return id;
  }
  function createGroup({ windowId = 1, title = DELIVERABLE_TITLE } = {}) {
    const id = state.nextGroupId++;
    state.groups.set(id, { id, windowId, title, color: "green", collapsed: false });
    return id;
  }
  function setTabGroup(tabId, groupId) {
    const t = state.tabs.get(tabId);
    if (!t) throw new Error(`Unknown tab ${tabId}`);
    t.groupId = groupId;
  }
  function listGroupsByTitle(title, windowId) {
    return [...state.groups.values()].filter(
      (g) => g.title === title && (windowId == null || g.windowId === windowId)
    );
  }
  function tabsInGroup(groupId) {
    return [...state.tabs.values()].filter((t) => t.groupId === groupId).map((t) => t.id);
  }

  return { chrome, state, helpers: { createTab, createGroup, setTabGroup, listGroupsByTitle, tabsInGroup } };
}

async function loadController(bgSource, chromeFake) {
  // Stop background.js from auto-instantiating its main classes by trimming
  // top-level bootstrap. We rebuild minimal pieces from the exports we need.
  // Simplest path: evaluate the file with a sandbox that exposes the classes
  // we need via globalThis hooks we inject before evaluation.
  const inject = `
globalThis.__obu_export = (name, value) => { globalThis[name] = value; };
`;
  // Append exporter at end so we capture symbols after class definitions.
  const exporter = `
__obu_export("SessionStore", SessionStore);
__obu_export("BrowserBackend", BrowserBackend);
__obu_export("DELIVERABLE_GROUP_TITLE", DELIVERABLE_GROUP_TITLE);
`;
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
    crypto: globalThis.crypto
  });
  vm.runInContext(inject, context);
  // We can't easily prevent background.js bootstrap from running other code,
  // but we can survive it by stubbing the symbols it needs. The file does set
  // up listeners at module load — our chrome fake supplies addListener stubs.
  // It also tries to connect a native messaging port: stub it.
  vm.runInContext(
    "chrome.runtime.connectNative = () => ({ onMessage: { addListener(){} }, onDisconnect: { addListener(){} }, postMessage(){}, disconnect(){} });",
    context
  );
  vm.runInContext(bgSource + "\n" + exporter, context);
  return {
    SessionStore: context.SessionStore,
    BrowserBackend: context.BrowserBackend,
    DELIVERABLE_GROUP_TITLE: context.DELIVERABLE_GROUP_TITLE
  };
}

function makeController(SessionStore, BrowserBackend) {
  const store = new SessionStore();
  // BrowserBackend constructor signature is unknown; build a minimal stub
  // that only relies on `this.store`.
  const controller = Object.create(BrowserBackend.prototype);
  controller.store = store;
  controller.activeTabsBySession = new Map();
  return { controller, store };
}

async function run() {
  const bgSource = await readFile(BG_PATH, "utf8");

  // === Test 1: existing deliverable group is reused (no duplicate created) ===
  {
    const { chrome, helpers } = createChromeFake();
    const { SessionStore, BrowserBackend } = await loadController(bgSource, chrome);
    const { controller, store } = makeController(SessionStore, BrowserBackend);
    await store.ready;

    // Simulate session-restored state: existing tab already in a deliverable
    // group with id 100, but our stored deliverableGroupId is null.
    const existingGroupId = helpers.createGroup({ windowId: 1, title: DELIVERABLE_TITLE });
    const restoredTab = helpers.createTab({ windowId: 1, groupId: existingGroupId });
    const newTab = helpers.createTab({ windowId: 1, groupId: -1 });

    await controller.moveToDeliverables([newTab]);

    const groups = helpers.listGroupsByTitle(DELIVERABLE_TITLE, 1);
    assert.equal(groups.length, 1, `expected 1 deliverable group, got ${groups.length}`);
    assert.equal(groups[0].id, existingGroupId, "should reuse existing group, not create a new one");
    const members = helpers.tabsInGroup(existingGroupId).sort();
    assert.deepEqual(members.sort(), [restoredTab, newTab].sort(), "new tab should be added to the existing group");
    console.log("test 1 ok: reused existing deliverable group");
  }

  // === Test 2: two stale duplicate groups get merged into one ===
  {
    const { chrome, helpers } = createChromeFake();
    const { SessionStore, BrowserBackend } = await loadController(bgSource, chrome);
    const { controller, store } = makeController(SessionStore, BrowserBackend);
    await store.ready;

    // Reproduce the exact bug state described by the user: two groups with
    // the same deliverable title, each holding a tab.
    const groupA = helpers.createGroup({ windowId: 1, title: DELIVERABLE_TITLE });
    const groupB = helpers.createGroup({ windowId: 1, title: DELIVERABLE_TITLE });
    const tabA = helpers.createTab({ windowId: 1, groupId: groupA });
    const tabB = helpers.createTab({ windowId: 1, groupId: groupB });
    const newTab = helpers.createTab({ windowId: 1, groupId: -1 });

    await controller.moveToDeliverables([newTab]);

    const groups = helpers.listGroupsByTitle(DELIVERABLE_TITLE, 1);
    assert.equal(groups.length, 1, `expected groups to be merged to 1, got ${groups.length}`);
    const survivor = groups[0].id;
    const members = helpers.tabsInGroup(survivor).sort();
    assert.deepEqual(members.sort(), [tabA, tabB, newTab].sort(), "all tabs should land in the merged group");
    console.log("test 2 ok: merged duplicate deliverable groups");
  }

  // === Test 3: no existing group at all -> creates one ===
  {
    const { chrome, helpers } = createChromeFake();
    const { SessionStore, BrowserBackend } = await loadController(bgSource, chrome);
    const { controller, store } = makeController(SessionStore, BrowserBackend);
    await store.ready;

    const tab = helpers.createTab({ windowId: 1, groupId: -1 });
    await controller.moveToDeliverables([tab]);

    const groups = helpers.listGroupsByTitle(DELIVERABLE_TITLE, 1);
    assert.equal(groups.length, 1, `expected 1 newly-created deliverable group, got ${groups.length}`);
    assert.deepEqual(helpers.tabsInGroup(groups[0].id), [tab]);
    console.log("test 3 ok: created new deliverable group when none existed");
  }

  // === Test 4: per-window isolation — two windows with deliverable groups, each gets its own ===
  {
    const { chrome, helpers } = createChromeFake();
    const { SessionStore, BrowserBackend } = await loadController(bgSource, chrome);
    const { controller, store } = makeController(SessionStore, BrowserBackend);
    await store.ready;

    const win1Group = helpers.createGroup({ windowId: 1, title: DELIVERABLE_TITLE });
    helpers.createTab({ windowId: 1, groupId: win1Group });
    const tab2 = helpers.createTab({ windowId: 2, groupId: -1 });

    await controller.moveToDeliverables([tab2]);

    const win1Groups = helpers.listGroupsByTitle(DELIVERABLE_TITLE, 1);
    const win2Groups = helpers.listGroupsByTitle(DELIVERABLE_TITLE, 2);
    assert.equal(win1Groups.length, 1, "window 1 should still have exactly 1 group");
    assert.equal(win2Groups.length, 1, "window 2 should have its own new group");
    assert.notEqual(win1Groups[0].id, win2Groups[0].id, "windows must have distinct group ids");
    console.log("test 4 ok: per-window grouping respected");
  }

  console.log("\nAll moveToDeliverables tests passed.");
}

run().catch((err) => {
  console.error(err);
  process.exit(1);
});
