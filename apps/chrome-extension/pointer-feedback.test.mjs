import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import vm from "node:vm";

const source = await readFile(new URL("./background.js", import.meta.url), "utf8");
const runtimeStart = source.indexOf("chrome.runtime.onInstalled.addListener");
assert.notEqual(runtimeStart, -1);
const session = { session_id: "pointer-test", turn_id: "turn-1" };
const json = (value) => JSON.parse(JSON.stringify(value));
const mouse = (type = "mouseMoved", extra = {}) => ({
  ...session, target: { tabId: 7 }, method: "Input.dispatchMouseEvent",
  commandParams: { type, x: 120, y: 80 }, ...extra
});
const deferred = () => {
  let resolve;
  const promise = new Promise((done) => { resolve = done; });
  return { promise, resolve };
};

async function loadBackend({ sendCommand, sendMessage, inject } = {}) {
  const calls = { commands: [], messages: [], injections: [], focus: [] };
  const result = { synthetic: "CDP result" };
  const chrome = {
    debugger: {
      onDetach: { addListener() {} },
      async detach() {},
      async sendCommand(...args) {
        calls.commands.push(json(args));
        return sendCommand ? await sendCommand(...args) : result;
      }
    },
    storage: { local: { async get() { return {}; }, async set() {} } },
    tabGroups: { async get() { return { id: 10 }; } },
    tabs: {
      async query() { return [{ id: 7, groupId: 10, url: "https://example.test" }]; },
      async update(...args) { calls.focus.push(args); },
      async sendMessage(tabId, message) {
        calls.messages.push({ tabId, ...json(message) });
        return sendMessage ? await sendMessage(tabId, message) : { ok: true };
      }
    },
    windows: { async update(...args) { calls.focus.push(args); } },
    scripting: {
      async executeScript(options) {
        calls.injections.push(json(options));
        return inject?.(options);
      }
    }
  };
  const BrowserBackend = vm.runInNewContext(
    `${source.slice(0, runtimeStart)}\nBrowserBackend;`,
    { chrome, Date, setTimeout, clearTimeout }
  );
  const backend = new BrowserBackend();
  await backend.store.ready;
  const state = await backend.store.getSession(session.session_id);
  state.chromeGroupId = 10;
  state.activeTabId = 7;
  backend.activeTabsBySession.set(session.session_id, 7);
  backend.attachedTabs.add(7);
  return { backend, calls, result };
}

async function flushFeedback(backend) {
  await Promise.all([...backend.pointerFeedbackByTabId.values()].map((queue) => queue.running));
}

for (const type of ["mouseMoved", "mousePressed", "mouseReleased", "mouseWheel"]) {
  test(`raw ${type} updates the page pointer without focusing the browser`, async () => {
    const { backend, calls, result } = await loadBackend();
    assert.equal(await backend.executeCdp(mouse(type)), result);
    await flushFeedback(backend);
    assert.deepEqual(calls.commands, [[{ tabId: 7 }, "Input.dispatchMouseEvent", { type, x: 120, y: 80 }]]);
    const updates = calls.messages.filter(({ type }) => type !== "OPEN_BROWSER_USE_PING");
    assert.deepEqual(updates[0], {
      tabId: 7, type: "OPEN_BROWSER_USE_CURSOR", sessionId: session.session_id,
      turnId: session.turn_id, moveSequence: 1, animateMovement: false,
      visible: true, x: 120, y: 80
    });
    assert.equal(updates.length, type === "mousePressed" ? 2 : 1);
    if (type === "mousePressed") {
      assert.deepEqual(updates[1], {
        tabId: 7, type: "OPEN_BROWSER_USE_POINTER_EVENT", sessionId: session.session_id,
        turnId: session.turn_id, eventType: type, x: 120, y: 80
      });
    }
    assert.equal(backend.cursorArrivalWaitersByKey.size, 0);
    assert.deepEqual(calls.focus, []);
  });
}

test("raw input reuses an existing move sequence at the same point and reports every press", async () => {
  const { backend, calls } = await loadBackend();
  await backend.moveMouse({ ...session, tabId: 7, x: 120, y: 80, waitForArrival: false });
  const sequence = backend.cursorByTabId.get(7).moveSequence;
  await backend.executeCdp(mouse("mousePressed"));
  await backend.executeCdp(mouse("mouseReleased"));
  await backend.executeCdp(mouse("mousePressed"));
  await flushFeedback(backend);
  assert.equal(backend.cursorByTabId.get(7).moveSequence, sequence);
  assert.equal(calls.messages.filter(({ type }) => type === "OPEN_BROWSER_USE_POINTER_EVENT").length, 2);
});

test("non-pointer, invalid coordinate and child-session commands do not guess a top-level position", async () => {
  const { backend, calls } = await loadBackend();
  for (const params of [
    { ...mouse(), method: "Input.insertText", commandParams: { text: "example" } },
    mouse("mouseMoved", { target: { tabId: 7, sessionId: "child-frame" } }),
    mouse("mouseMoved", { target: { targetId: "other-target" } }),
    ...[NaN, Infinity, "120", undefined].map((x) => mouse("mouseMoved", { commandParams: { type: "mouseMoved", x, y: 80 } })),
    mouse("unknown"),
    mouse("mouseMoved", { commandParams: undefined })
  ]) {
    await backend.executeCdp(params);
  }
  assert.deepEqual(calls.messages, []);
  assert.equal(backend.cursorByTabId.size, 0);
});

test("CDP failures and unauthorized tabs never display a successful input", async () => {
  const failure = new Error("Input rejected");
  const { backend, calls } = await loadBackend({ sendCommand: async () => { throw failure; } });
  await assert.rejects(backend.executeCdp(mouse()), (error) => error === failure);
  await assert.rejects(backend.executeCdp(mouse("mouseMoved", { target: { tabId: 99 } })), /not part/);
  assert.equal(calls.commands.length, 1);
  assert.deepEqual(calls.messages, []);
});

test("restricted pages and failed feedback keep the original successful CDP result", async () => {
  for (const options of [
    { sendMessage: async () => { throw new Error("No receiver"); }, inject: async () => { throw new Error("Restricted"); } },
    { sendMessage: async (_id, message) => { if (message.type !== "OPEN_BROWSER_USE_PING") throw new Error("Navigated"); return { ok: true }; } }
  ]) {
    const { backend, result, calls } = await loadBackend(options);
    assert.equal(await backend.executeCdp(mouse("mousePressed")), result);
    await flushFeedback(backend);
    assert.equal(calls.commands.length, 1);
  }
});

test("an unresponsive content script cannot consume the successful CDP request deadline", async () => {
  const ping = deferred();
  const { backend, calls, result } = await loadBackend({
    sendMessage: async (_id, message) => message.type === "OPEN_BROWSER_USE_PING" ? ping.promise : { ok: true }
  });
  let deadline;
  try {
    const response = await Promise.race([
      backend.executeCdp(mouse("mousePressed", { timeoutMs: 25 })),
      new Promise((_, reject) => {
        deadline = setTimeout(() => reject(new Error("Feedback delayed the input response")), 100);
      })
    ]);
    assert.equal(response, result);
  } finally {
    clearTimeout(deadline);
  }
  assert.equal(calls.commands.length, 1);
  await flushFeedback(backend);
  ping.resolve({ ok: true });
  await new Promise(setImmediate);
  assert.deepEqual(calls.messages.map(({ type }) => type), ["OPEN_BROWSER_USE_PING"]);
});

test("queued press and release feedback preserves clicks without serializing input", async () => {
  const ping = deferred();
  const started = deferred();
  let first = true;
  const { backend, calls } = await loadBackend({ sendMessage: async (_id, message) => {
    if (message.type === "OPEN_BROWSER_USE_PING" && first) {
      first = false;
      started.resolve();
      return ping.promise;
    }
    return { ok: true };
  } });
  await backend.executeCdp(mouse("mousePressed"));
  await started.promise;
  await backend.executeCdp(mouse("mouseReleased"));
  await backend.executeCdp(mouse("mouseMoved", { commandParams: { type: "mouseMoved", x: 240, y: 160 } }));
  assert.equal(calls.commands.length, 3);
  assert.equal(calls.messages.some(({ type }) => type === "OPEN_BROWSER_USE_CURSOR"), false);
  ping.resolve({ ok: true });
  await flushFeedback(backend);
  const updates = calls.messages.filter(({ type }) => type === "OPEN_BROWSER_USE_CURSOR");
  assert.deepEqual(updates.map(({ x }) => x), [120, 120, 240]);
  assert.equal(calls.messages.filter(({ type }) => type === "OPEN_BROWSER_USE_POINTER_EVENT").length, 1);
  assert.equal(backend.cursorByTabId.get(7).x, 240);
});

test("turn end clears feedback and suppresses an in-flight update", async () => {
  const ping = deferred();
  const started = deferred();
  let first = true;
  const { backend, calls } = await loadBackend({ sendMessage: async (_id, message) => {
    if (message.type === "OPEN_BROWSER_USE_PING" && first) {
      first = false;
      started.resolve();
      return ping.promise;
    }
    return { ok: true };
  } });
  const input = backend.executeCdp(mouse("mousePressed"));
  await started.promise;
  await backend.executeCdp(mouse("mouseReleased"));
  const queue = backend.pointerFeedbackByTabId.get(7);
  await backend.turnEnded(session);
  assert.equal(backend.pointerFeedbackByTabId.size, 0);
  ping.resolve({ ok: true });
  await input;
  await queue.running;
  assert.equal(backend.cursorByTabId.size, 0);
  const updates = calls.messages.filter(({ type }) => type !== "OPEN_BROWSER_USE_PING");
  assert.deepEqual(updates, [{ tabId: 7, type: "OPEN_BROWSER_USE_CURSOR_STATE", state: {
    cursor: null, isVisible: false, sessionId: null, turnId: null
  } }]);
});

test("feedback backlog is bounded and adjacent moves keep only the latest position", async () => {
  const ping = deferred();
  let first = true;
  const { backend, calls } = await loadBackend({ sendMessage: async (_id, message) => {
    if (message.type === "OPEN_BROWSER_USE_PING" && first) {
      first = false;
      return ping.promise;
    }
    return { ok: true };
  } });
  await backend.executeCdp(mouse("mousePressed"));
  const queue = backend.pointerFeedbackByTabId.get(7);
  for (let index = 0; index < 20; index++) {
    await backend.executeCdp(mouse("mousePressed", { commandParams: { type: "mousePressed", x: index, y: 80 } }));
    assert.ok(queue.pending.length <= 7);
  }
  for (let index = 0; index < 20; index++) {
    await backend.executeCdp(mouse("mouseMoved", { commandParams: { type: "mouseMoved", x: 200 + index, y: 80 } }));
    assert.ok(queue.pending.length <= 7);
  }
  assert.equal(calls.commands.length, 41);
  assert.equal(queue.pending.filter(({ commandParams }) => commandParams.type === "mouseMoved").length, 1);
  ping.resolve({ ok: true });
  await flushFeedback(backend);
  assert.equal(backend.cursorByTabId.get(7).x, 219);
  assert.equal(calls.messages.filter(({ type }) => type === "OPEN_BROWSER_USE_CURSOR").length, 8);
  assert.equal(backend.pointerFeedbackByTabId.size, 0);
});

test("a detached queue cannot revive after a new attachment creates another queue", async () => {
  const ping = deferred();
  let first = true;
  const { backend, calls } = await loadBackend({ sendMessage: async (_id, message) => {
    if (message.type === "OPEN_BROWSER_USE_PING" && first) {
      first = false;
      return ping.promise;
    }
    return { ok: true };
  } });
  await backend.executeCdp(mouse("mousePressed"));
  const oldQueue = backend.pointerFeedbackByTabId.get(7);
  await backend.detachTab(7);
  backend.attachedTabs.add(7);
  await backend.executeCdp(mouse("mouseMoved", { commandParams: { type: "mouseMoved", x: 240, y: 160 } }));
  await flushFeedback(backend);
  ping.resolve({ ok: true });
  await oldQueue.running;
  const updates = calls.messages.filter(({ type }) => type === "OPEN_BROWSER_USE_CURSOR");
  assert.deepEqual(updates.map(({ x }) => x), [240]);
  assert.equal(calls.messages.some(({ type }) => type === "OPEN_BROWSER_USE_POINTER_EVENT"), false);
  assert.equal(backend.cursorByTabId.get(7).x, 240);
});
