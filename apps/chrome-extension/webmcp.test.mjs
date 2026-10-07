import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import vm from "node:vm";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const BACKGROUND_PATH = path.join(HERE, "background.js");
const MAIN_PATH = path.join(HERE, "content-webmcp-main.js");
const BRIDGE_PATH = path.join(HERE, "content-webmcp-bridge.js");

function json(value) {
  return JSON.parse(JSON.stringify(value));
}

function createWindowBus() {
  const listeners = new Set();
  const window = {
    addEventListener(type, listener) {
      if (type === "message") listeners.add(listener);
    },
    postMessage(data) {
      const cloned = structuredClone(data);
      queueMicrotask(() => {
        for (const listener of listeners) {
          listener({ source: window, data: cloned });
        }
      });
    }
  };
  return window;
}

function createRuntimeFake() {
  const listeners = [];
  const runtime = {
    onMessage: {
      addListener(listener) {
        listeners.push(listener);
      }
    }
  };
  runtime.sendToContent = async (message) => {
    return await new Promise((resolve, reject) => {
      let accepted = false;
      let responded = false;
      const sendResponse = (response) => {
        responded = true;
        resolve(response);
      };
      for (const listener of listeners) {
        const result = listener(message, {}, sendResponse);
        if (responded) return;
        if (result === true) {
          accepted = true;
          break;
        }
      }
      if (!accepted && !responded) {
        reject(new Error(`No content listener accepted ${message.type}`));
      }
    });
  };
  return runtime;
}

function createModelContext(tools) {
  const listeners = new Set();
  return {
    async getTools() {
      return tools;
    },
    async executeTool(tool, inputJson, options) {
      return await tool.__execute(JSON.parse(inputJson), options);
    },
    addEventListener(type, listener) {
      if (type === "toolchange") listeners.add(listener);
    },
    removeEventListener(type, listener) {
      if (type === "toolchange") listeners.delete(listener);
    },
    emitToolChange() {
      for (const listener of listeners) listener();
    }
  };
}

async function loadBridgeHarness({ modelContext }) {
  const [mainSource, bridgeSource] = await Promise.all([
    readFile(MAIN_PATH, "utf8"),
    readFile(BRIDGE_PATH, "utf8")
  ]);
  const window = createWindowBus();
  const runtime = createRuntimeFake();
  const common = {
    AbortController,
    clearTimeout,
    console,
    crypto: globalThis.crypto,
    location: {
      href: "https://fixture.example.test/demo",
      origin: "https://fixture.example.test"
    },
    queueMicrotask,
    setTimeout,
    structuredClone,
    window
  };
  const document = { modelContext };
  const mainContext = vm.createContext({
    ...common,
    document
  });
  const bridgeContext = vm.createContext({
    ...common,
    chrome: { runtime }
  });
  vm.runInContext(mainSource, mainContext, { filename: MAIN_PATH });
  vm.runInContext(bridgeSource, bridgeContext, { filename: BRIDGE_PATH });
  return { document, runtime };
}

async function loadBrowserBackend() {
  const source = await readFile(BACKGROUND_PATH, "utf8");
  const runtimeStart = source.indexOf("chrome.runtime.onInstalled.addListener");
  assert.notEqual(runtimeStart, -1);
  const chrome = {
    debugger: {
      onDetach: { addListener() {} }
    },
    runtime: {
      getManifest() {
        return { version: "0.0.0-test" };
      },
      id: "test-extension"
    },
    storage: {
      local: {
        async get() {
          return {};
        },
        async set() {}
      },
      onChanged: { addListener() {} }
    }
  };
  const context = vm.createContext({
    chrome,
    clearTimeout,
    console,
    crypto: globalThis.crypto,
    Date,
    setTimeout,
    structuredClone,
    TextDecoder,
    TextEncoder,
    URL
  });
  vm.runInContext(
    `${source.slice(0, runtimeStart)}\nglobalThis.__BrowserBackend = BrowserBackend;`,
    context,
    { filename: BACKGROUND_PATH }
  );
  return context.__BrowserBackend;
}

test("WebMCP bridge lists a snapshot and invokes its registered tool", async () => {
  const registeredTool = {
    name: "book_table",
    title: "Book table",
    description: "Book a table at the fixture restaurant.",
    inputSchema: JSON.stringify({
      type: "object",
      properties: { partySize: { type: "integer" } },
      required: ["partySize"]
    }),
    annotations: {
      readOnlyHint: false,
      untrustedContentHint: true,
      unsupportedHint: "ignored"
    },
    origin: "https://fixture.example.test",
    async __execute(input, options) {
      assert.deepEqual(input, { partySize: 4 });
      assert.equal(options.signal.aborted, false);
      return JSON.stringify({ reservationId: "r-42", partySize: input.partySize });
    }
  };
  const modelContext = createModelContext([registeredTool]);
  const { runtime } = await loadBridgeHarness({ modelContext });

  assert.deepEqual(
    json(await runtime.sendToContent({ type: "OPEN_BROWSER_USE_WEBMCP_PING" })),
    { ok: true }
  );

  const listed = await runtime.sendToContent({
    type: "OPEN_BROWSER_USE_WEBMCP_REQUEST",
    action: "list"
  });
  assert.equal(listed.ok, true);
  const tool = json(listed.result.tools[0]);
  assert.equal(tool.name, "book_table");
  assert.equal(typeof tool.registration_id, "string");
  assert.deepEqual(tool.input_schema, {
    type: "object",
    properties: { partySize: { type: "integer" } },
    required: ["partySize"]
  });
  assert.deepEqual(tool.annotations, {
    readOnlyHint: false,
    untrustedContentHint: true
  });
  assert.equal(tool.origin, "https://fixture.example.test");
  assert.equal(tool.pageUrl, "https://fixture.example.test/demo");

  const invoked = await runtime.sendToContent({
    type: "OPEN_BROWSER_USE_WEBMCP_REQUEST",
    action: "invoke",
    tool_name: tool.name,
    registration_id: tool.registration_id,
    input: { partySize: 4 },
    timeout_ms: 1_000
  });
  assert.deepEqual(json(invoked), {
    ok: true,
    result: {
      result: { reservationId: "r-42", partySize: 4 }
    }
  });
});

test("WebMCP bridge rejects a registration after toolchange", async () => {
  const registeredTool = {
    name: "fixture_echo",
    inputSchema: { type: "object" },
    async __execute(input) {
      return input;
    }
  };
  const modelContext = createModelContext([registeredTool]);
  const { runtime } = await loadBridgeHarness({ modelContext });
  const listed = await runtime.sendToContent({
    type: "OPEN_BROWSER_USE_WEBMCP_REQUEST",
    action: "list"
  });
  const tool = listed.result.tools[0];

  modelContext.emitToolChange();
  const invoked = await runtime.sendToContent({
    type: "OPEN_BROWSER_USE_WEBMCP_REQUEST",
    action: "invoke",
    tool_name: tool.name,
    registration_id: tool.registration_id,
    input: {},
    timeout_ms: 1_000
  });

  assert.equal(invoked.ok, false);
  assert.match(invoked.error, /stale; list tools again/);
});

test("WebMCP bridge aborts a tool that exceeds timeout_ms", async () => {
  let aborted = false;
  const registeredTool = {
    name: "slow_tool",
    inputSchema: { type: "object" },
    async __execute(_input, { signal }) {
      signal.addEventListener("abort", () => {
        aborted = true;
      });
      return await new Promise(() => {});
    }
  };
  const modelContext = createModelContext([registeredTool]);
  const { runtime } = await loadBridgeHarness({ modelContext });
  const listed = await runtime.sendToContent({
    type: "OPEN_BROWSER_USE_WEBMCP_REQUEST",
    action: "list"
  });
  const tool = listed.result.tools[0];

  const invoked = await runtime.sendToContent({
    type: "OPEN_BROWSER_USE_WEBMCP_REQUEST",
    action: "invoke",
    tool_name: tool.name,
    registration_id: tool.registration_id,
    input: {},
    timeout_ms: 10
  });

  assert.equal(invoked.ok, false);
  assert.match(invoked.error, /timed out after 10ms/);
  assert.equal(aborted, true);
});

test("WebMCP MAIN shim captures page tools when the browser has no modelContext", async () => {
  const { document, runtime } = await loadBridgeHarness({ modelContext: undefined });
  assert.equal(typeof document.modelContext?.registerTool, "function");

  const registration = new AbortController();
  await document.modelContext.registerTool(
    {
      name: "search_fixture",
      description: "Search the fixture data.",
      inputSchema: {
        type: "object",
        properties: { query: { type: "string" } },
        required: ["query"]
      },
      annotations: { readOnlyHint: true, untrustedContentHint: false },
      async execute(input, { signal }) {
        assert.equal(signal.aborted, false);
        return { query: input.query, status: "ok" };
      }
    },
    { signal: registration.signal }
  );

  const listed = await runtime.sendToContent({
    type: "OPEN_BROWSER_USE_WEBMCP_REQUEST",
    action: "list"
  });
  assert.equal(listed.ok, true);
  const tool = json(listed.result.tools[0]);
  assert.equal(tool.name, "search_fixture");
  assert.deepEqual(tool.annotations, { readOnlyHint: true, untrustedContentHint: false });
  assert.deepEqual(tool.input_schema.required, ["query"]);

  const invoked = await runtime.sendToContent({
    type: "OPEN_BROWSER_USE_WEBMCP_REQUEST",
    action: "invoke",
    tool_name: tool.name,
    registration_id: tool.registration_id,
    input: { query: "test-query" },
    timeout_ms: 1_000
  });
  assert.deepEqual(json(invoked), {
    ok: true,
    result: { result: { query: "test-query", status: "ok" } }
  });

  registration.abort();
  const afterAbort = await runtime.sendToContent({
    type: "OPEN_BROWSER_USE_WEBMCP_REQUEST",
    action: "list"
  });
  assert.deepEqual(json(afterAbort), { ok: true, result: { tools: [] } });
});

test("WebMCP MAIN shim starts empty before a page registers tools", async () => {
  const { document, runtime } = await loadBridgeHarness({ modelContext: undefined });
  assert.equal(typeof document.modelContext?.getTools, "function");
  const listed = await runtime.sendToContent({
    type: "OPEN_BROWSER_USE_WEBMCP_REQUEST",
    action: "list"
  });
  assert.deepEqual(json(listed), { ok: true, result: { tools: [] } });
});

test("WebMCP backend keeps obu session routing and forwards the standard WebMCP contract", async () => {
  const BrowserBackend = await loadBrowserBackend();
  const gate = { async enabled() { return true; } };
  const backend = new BrowserBackend({ webMcpGate: gate });
  const guarded = [];
  const requests = [];
  backend.requireSessionTab = async (params, command) => {
    guarded.push({ params: json(params), command });
  };
  backend.sendWebMcpRequest = async (tabId, request, timeoutMs) => {
    requests.push({ tabId, request: json(request), timeoutMs });
    if (request.action === "list") {
      return {
        tools: [
          {
            name: "fixture_echo",
            registration_id: "registration-1",
            input_schema: { type: "object" }
          }
        ]
      };
    }
    return { result: { echoed: request.input.message } };
  };

  const listed = await backend.webmcp_list_tools({
    session_id: "session-1",
    turn_id: "turn-1",
    tab_id: "321"
  });
  assert.equal(listed.tools[0].registration_id, "registration-1");
  assert.equal(guarded[0].params.tabId, 321);
  assert.equal(guarded[0].command, "webmcp_list_tools");
  assert.deepEqual(requests[0], {
    tabId: 321,
    request: { action: "list" },
    timeoutMs: 6_500
  });

  const invoked = await backend.webmcp_invoke_tool({
    session_id: "session-1",
    turn_id: "turn-1",
    tabId: 321,
    tool_name: " fixture_echo ",
    tool_title: "Fixture echo",
    tool_description: "Echo fixture input.",
    registration_id: " registration-1 ",
    input: { message: "hello" },
    timeout_ms: 2_000
  });
  assert.deepEqual(json(invoked), { result: { echoed: "hello" } });
  assert.deepEqual(requests[1], {
    tabId: 321,
    request: {
      action: "invoke",
      tool_name: "fixture_echo",
      tool_title: "Fixture echo",
      tool_description: "Echo fixture input.",
      registration_id: "registration-1",
      input: { message: "hello" },
      timeout_ms: 2_000
    },
    timeoutMs: 3_500
  });

  const info = json(await backend.getInfo());
  assert.deepEqual(info.capabilities.tab, [
    {
      id: "webmcp",
      description: "Fetch page-defined WebMCP tools bound to the current document and invoke them."
    }
  ]);
});

test("WebMCP backend rejects calls when its gate is inactive", async () => {
  const BrowserBackend = await loadBrowserBackend();
  const backend = new BrowserBackend({ webMcpGate: { async enabled() { return false; } } });
  backend.requireSessionTab = async () => {};
  backend.sendWebMcpRequest = async () => {
    throw new Error("request must not be sent");
  };

  await assert.rejects(
    backend.webmcp_list_tools({
      session_id: "session-1",
      turn_id: "turn-1",
      tabId: 321
    }),
    /WebMCP capability is disabled/
  );
  const info = json(await backend.getInfo());
  assert.deepEqual(info.capabilities.tab, []);
});
