import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import vm from "node:vm";

const source = await readFile(new URL("./background.js", import.meta.url), "utf8");
const runtimeStart = source.indexOf("chrome.runtime.onInstalled.addListener");
assert.notEqual(runtimeStart, -1);
const session = { session_id: "history-test", turn_id: "turn-1" };
const json = (value) => JSON.parse(JSON.stringify(value));

async function loadBackend(search = async () => []) {
  const calls = [];
  const chrome = {
    debugger: { onDetach: { addListener() {} } },
    storage: { local: { async get() { return {}; }, async set() {} } },
    history: {
      async search(params) {
        calls.push(json(params));
        return await search(params);
      }
    }
  };
  const BrowserBackend = vm.runInNewContext(
    `${source.slice(0, runtimeStart)}\nBrowserBackend;`,
    { chrome, Date }
  );
  const backend = new BrowserBackend();
  await backend.store.ready;
  return { backend, calls };
}

function entry(url, date, title) {
  return { url, lastVisitTime: Date.parse(date), ...(title ? { title } : {}) };
}

test("history merges queries, keeps the newest URL entry, and applies one limit", async () => {
  const results = {
    guide: [
      entry("https://example.test/shared", "2026-09-01", "Old title"),
      entry("https://example.test/guide", "2026-09-02", "Guide")
    ],
    api: [
      entry("https://example.test/shared", "2026-09-06", "New title"),
      entry("https://example.test/api", "2026-09-05", "API")
    ]
  };
  const { backend, calls } = await loadBackend(async ({ text }) => results[text] ?? []);
  const result = await backend.getUserHistory({ ...session, queries: ["guide", "api"], limit: 2 });
  assert.deepEqual(json(result), [
    { url: "https://example.test/shared", title: "New title", dateVisited: "2026-09-06T00:00:00.000Z" },
    { url: "https://example.test/api", title: "API", dateVisited: "2026-09-05T00:00:00.000Z" }
  ]);
  assert.deepEqual(calls, [
    { text: "guide", maxResults: 2, startTime: 0 },
    { text: "api", maxResults: 2, startTime: 0 }
  ]);
});

test("history keeps the query alias and sends the requested date range", async () => {
  const { backend, calls } = await loadBackend();
  await backend.getUserHistory({
    ...session, query: "a phrase, with commas", limit: 7,
    from: "2026-08-01T00:00:00Z", to: "2026-09-01T00:00:00Z"
  });
  assert.deepEqual(calls, [{
    text: "a phrase, with commas", maxResults: 7,
    startTime: Date.parse("2026-08-01T00:00:00Z"),
    endTime: Date.parse("2026-09-01T00:00:00Z")
  }]);
});

test("history starts at the epoch when no from date is supplied", async () => {
  const { backend, calls } = await loadBackend();
  await backend.getUserHistory(session);
  assert.deepEqual(calls, [{ text: "", maxResults: 100, startTime: 0 }]);
});

test("queries take precedence over query and repeated queries run once", async () => {
  const { backend, calls } = await loadBackend();
  await backend.getUserHistory({ ...session, query: "ignored", queries: ["api", "api", ""] });
  assert.deepEqual(calls.map(({ text }) => text), ["api", ""]);
});

test("history rejects invalid inputs before searching Chrome", async () => {
  const { backend, calls } = await loadBackend();
  const invalid = [
    [{ queries: [] }, /queries/],
    [{ queries: "api" }, /queries/],
    [{ queries: ["api", 42] }, /queries/],
    [{ queries: [null] }, /queries/],
    [{ query: 42 }, /query/],
    ...[0, -1, 1.5, "10", Infinity].map((limit) => [{ limit }, /limit/]),
    [{ from: 42 }, /from/],
    [{ from: "bad-date" }, /from/],
    [{ to: false }, /to/],
    [{ to: "bad-date" }, /to/],
    [{ from: "2026-09-06", to: "2026-09-01" }, /to/]
  ];
  for (const [params, message] of invalid) {
    await assert.rejects(backend.getUserHistory({ ...session, ...params }), message);
  }
  assert.equal(calls.length, 0);
});

test("history skips invalid rows and sorts valid results by visit time", async () => {
  const { backend } = await loadBackend(async () => [
    null,
    {},
    { url: "https://example.test/bad", lastVisitTime: "2026-09-01" },
    ...[NaN, Infinity, 1e20].map((lastVisitTime) => ({ url: "https://example.test/bad", lastVisitTime })),
    { url: 42, lastVisitTime: 0 },
    entry("https://example.test/older", "2026-08-01"),
    entry("https://example.test/newer", "2026-09-01")
  ]);
  const result = await backend.getUserHistory(session);
  assert.deepEqual(json(result), [
    { url: "https://example.test/newer", dateVisited: "2026-09-01T00:00:00.000Z" },
    { url: "https://example.test/older", dateVisited: "2026-08-01T00:00:00.000Z" }
  ]);
});

test("history reports a failed query instead of returning partial results", async () => {
  const failure = new Error("History service unavailable");
  const { backend } = await loadBackend(async ({ text }) => {
    if (text === "bad") throw failure;
    return [entry("https://example.test/page", "2026-09-01")];
  });
  await assert.rejects(
    backend.getUserHistory({ ...session, queries: ["good", "bad"] }),
    (error) => error === failure
  );
});

test("history requires session and turn metadata", async () => {
  const { backend, calls } = await loadBackend();
  await assert.rejects(backend.getUserHistory({ turn_id: "turn-1" }), /session_id/);
  await assert.rejects(backend.getUserHistory({ session_id: "history-test" }), /turn_id/);
  assert.equal(calls.length, 0);
});
