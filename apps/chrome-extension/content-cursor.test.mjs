import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";
import vm from "node:vm";

const source = await readFile(new URL("./content-cursor.js", import.meta.url), "utf8");

function createPage({ reducedMotion = false } = {}) {
  const timers = new Map();
  const frames = new Map();
  const listeners = new Map();
  const animations = [];
  const sent = [];
  let nextTimer = 1;
  let timestamp = 0;
  let onMessage;
  let resolveInitialState;
  let hitElements = [];

  class Element {
    constructor(tagName) {
      this.tagName = tagName.toUpperCase();
      this.style = {};
      this.dataset = {};
      this.attributes = new Map();
      this.children = [];
      this.parentElement = null;
      this.rect = { left: 10, top: 20, width: 80, height: 40 };
    }
    get isConnected() {
      return this === document.documentElement || this.parentElement?.isConnected === true;
    }
    setAttribute(name, value) { this.attributes.set(name, value); }
    appendChild(child) {
      child.remove();
      child.parentElement = this;
      this.children.push(child);
      return child;
    }
    replaceChildren() {
      for (const child of [...this.children]) child.remove();
    }
    remove() {
      if (this.parentElement) {
        const children = this.parentElement.children;
        children.splice(children.indexOf(this), 1);
        this.parentElement = null;
      }
    }
    contains(element) {
      return this === element || this.children.some((child) => child.contains(element));
    }
    closest(selector) {
      const selectors = selector.split(",");
      for (let element = this; element; element = element.parentElement) {
        if (selectors.some((part) => {
          if (part === element.tagName.toLowerCase()) return true;
          if (part === "a[href]") return element.tagName === "A" && element.attributes.has("href");
          if (part.startsWith('[role="')) return element.attributes.get("role") === part.slice(7, -2);
          if (part.startsWith("[contenteditable]")) {
            return element.attributes.has("contenteditable") && element.attributes.get("contenteditable") !== "false";
          }
          return part.startsWith("[tabindex]") && element.attributes.has("tabindex") && element.attributes.get("tabindex") !== "-1";
        })) return element;
      }
      return null;
    }
    getBoundingClientRect() { return this.rect; }
    animate(keyframes, options) {
      const animation = { keyframes, options, canceled: false, cancel() { this.canceled = true; } };
      animations.push(animation);
      return animation;
    }
    focus() { throw new Error("Overlay must not focus the page"); }
  }
  const document = {
    createElement: (tagName) => new Element(tagName),
    getElementById: (id) => findAll(document.documentElement, (element) => element.id === id)[0] ?? null,
    elementsFromPoint: () => hitElements
  };
  document.documentElement = new Element("html");
  document.body = document.documentElement.appendChild(new Element("body"));
  const addListener = (type, callback) => {
    const callbacks = listeners.get(type) ?? [];
    callbacks.push(callback);
    listeners.set(type, callbacks);
  };
  const window = {
    innerWidth: 1000,
    innerHeight: 800,
    addEventListener: addListener,
    visualViewport: { width: 1000, height: 800, addEventListener: addListener },
    matchMedia: () => ({ matches: reducedMotion }),
    setTimeout: (callback, delay) => {
      const id = nextTimer++;
      timers.set(id, { callback, due: timestamp + delay });
      return id;
    },
    clearTimeout: (id) => timers.delete(id),
    requestAnimationFrame: (callback) => {
      const id = nextTimer++;
      frames.set(id, callback);
      return id;
    },
    cancelAnimationFrame: (id) => frames.delete(id)
  };
  const context = vm.createContext({
    document, window,
    performance: { now: () => timestamp },
    chrome: {
      runtime: {
        onMessage: { addListener: (callback) => { onMessage = callback; } },
        getURL: (path) => `chrome-extension://test/${path}`,
        sendMessage: (message) => {
          sent.push(message);
          if (message.type === "GET_OPEN_BROWSER_USE_CURSOR_STATE") {
            return new Promise((resolve) => { resolveInitialState = resolve; });
          }
          return Promise.resolve({ ok: true });
        }
      }
    }
  });
  vm.runInContext(source, context);
  const page = {
    document, timers, frames, animations, sent,
    elements: (testId) => findAll(document.documentElement, (element) => element.dataset.testid === testId),
    add: (tagName, parent = document.body) => parent.appendChild(document.createElement(tagName)),
    hit: (...elements) => { hitElements = elements; },
    send: (message) => {
      let response;
      assert.equal(onMessage(message, {}, (value) => { response = value; }), true);
      assert.equal(response.ok, true);
    },
    move: (extra = {}) => page.send({
      type: "OPEN_BROWSER_USE_CURSOR", x: 40, y: 30,
      sessionId: "test-session", turnId: "test-turn", moveSequence: 1, ...extra
    }),
    click: (extra = {}) => page.send({
      type: "OPEN_BROWSER_USE_POINTER_EVENT", x: 40, y: 30,
      sessionId: "test-session", turnId: "test-turn", eventType: "mousePressed", ...extra
    }),
    hide: () => page.send({ type: "OPEN_BROWSER_USE_CURSOR_STATE", state: { isVisible: false } }),
    emit: (type) => { for (const callback of listeners.get(type) ?? []) callback(); },
    advance: (milliseconds) => {
      const end = timestamp + milliseconds;
      for (;;) {
        const next = [...timers].filter(([, item]) => item.due <= end).sort((a, b) => a[1].due - b[1].due)[0];
        if (!next) break;
        const [id, item] = next;
        timestamp = item.due;
        timers.delete(id);
        item.callback();
      }
      timestamp = end;
    },
    initialState: (state) => resolveInitialState({ ok: true, state })
  };
  return page;
}

function findAll(element, predicate) {
  return [element, ...element.children.flatMap((child) => findAll(child, () => true))].filter(predicate);
}

test("moving the AI cursor outlines the interactive ancestor without changing page elements", () => {
  const page = createPage();
  const button = page.add("button");
  const label = page.add("span", button);
  label.rect = { left: 30, top: 25, width: 20, height: 15 };
  button.style.background = "red";
  page.hit(label);
  page.move();
  const [outline] = page.elements("browser-agent-target");
  assert.equal(outline.style.display, "block");
  assert.equal(outline.style.left, "10px");
  assert.equal(outline.style.width, "80px");
  assert.equal(outline.style.pointerEvents, "none");
  assert.equal(documentStyle(page, "open-browser-use-cursor-root").pointerEvents, "none");
  assert.deepEqual(button.style, { background: "red" });
  assert.deepEqual(label.style, {});
  page.hide();
});

test("hit testing ignores overlay nodes and highlights an iframe without entering it", () => {
  const page = createPage();
  const frame = page.add("iframe");
  frame.rect = { left: 15, top: 25, width: 320, height: 180 };
  Object.defineProperty(frame, "contentDocument", { get() { throw new Error("Must not enter child frames"); } });
  page.move();
  const [cursor] = page.elements("browser-agent-cursor");
  page.hit(cursor, frame);
  page.move({ x: 90, y: 80, moveSequence: 2 });
  const [outline] = page.elements("browser-agent-target");
  assert.equal(outline.style.left, "15px");
  assert.equal(outline.style.width, "320px");
  page.hit(page.document.body);
  page.move();
  assert.equal(outline.style.display, "none");
  page.hide();
});

test("animateMovement false snaps to the CDP point and still acknowledges arrival", () => {
  const page = createPage();
  page.move({ x: 20, y: 25, animateMovement: false });
  page.move({ x: 620, y: 325, moveSequence: 2, animateMovement: false });
  const [cursor] = page.elements("browser-agent-cursor");
  assert.match(cursor.style.transform, /^translate3d\(608px, 313px, 0\)/);
  assert.equal(page.sent.filter((message) => message.type === "OPEN_BROWSER_USE_CURSOR_ARRIVED").at(-1).moveSequence, 2);
  page.hide();
});

test("nested open shadow roots highlight the actual button while opaque hosts stay opaque", () => {
  const page = createPage();
  const host = page.add("custom-controls");
  const shadow = page.document.createElement("#shadow-root");
  Object.defineProperty(shadow, "isConnected", { get: () => host.isConnected });
  host.shadowRoot = shadow;
  const nested = page.add("custom-button", shadow);
  const nestedShadow = page.document.createElement("#shadow-root");
  Object.defineProperty(nestedShadow, "isConnected", { get: () => nested.isConnected });
  nested.shadowRoot = nestedShadow;
  const button = page.add("button", nestedShadow);
  const label = page.add("span", button);
  button.rect = { left: 30, top: 25, width: 150, height: 35 };
  shadow.elementFromPoint = () => nested;
  nestedShadow.elementFromPoint = () => label;
  page.hit(host);
  page.move();
  const [outline] = page.elements("browser-agent-target");
  assert.equal(outline.style.left, "30px");
  assert.equal(outline.style.width, "150px");

  // Padding can hit the host again: stop instead of looping or hiding it.
  shadow.elementFromPoint = () => host;
  page.move();
  assert.equal(outline.style.width, "80px");
  host.shadowRoot = null;
  page.move();
  assert.equal(outline.style.width, "80px");
  page.hide();
});

test("rapid click effects are independent and do not restart cursor motion", () => {
  const page = createPage();
  page.move({ animateMovement: false });
  const [cursor] = page.elements("browser-agent-cursor");
  const transform = cursor.style.transform;
  const arrivals = page.sent.length;
  page.click();
  page.advance(100);
  page.click({ x: 100, y: 120 });
  assert.equal(page.elements("browser-agent-click").length, 2);
  assert.equal(cursor.style.transform, transform);
  assert.equal(page.sent.length, arrivals);
  assert.equal(page.elements("browser-agent-click")[1].style.pointerEvents, "none");
  page.advance(350);
  assert.equal(page.elements("browser-agent-click").length, 1);
  page.advance(100);
  assert.equal(page.elements("browser-agent-click").length, 0);
  assert.equal(page.animations.every((animation) => animation.canceled), true);
  page.hide();
  assert.equal(page.timers.size, 0);
  assert.equal(page.frames.size, 0);
});

test("reduced motion uses a brief static click marker", () => {
  const page = createPage({ reducedMotion: true });
  page.move();
  page.move({ x: 300, y: 200, moveSequence: 2 });
  assert.match(page.elements("browser-agent-cursor")[0].style.transform, /^translate3d\(288px, 188px, 0\)/);
  assert.equal(page.frames.size, 0);
  page.click();
  assert.equal(page.elements("browser-agent-click").length, 1);
  assert.equal(page.animations.length, 0);
  page.advance(200);
  assert.equal(page.elements("browser-agent-click").length, 0);
  page.hide();
});

test("new turns clear previous effects and rapid bursts keep a bounded set of markers", () => {
  const page = createPage();
  page.move({ animateMovement: false });
  for (let index = 0; index < 12; index++) page.click({ x: 40 + index });
  assert.equal(page.elements("browser-agent-click").length, 8);
  assert.equal(page.animations.filter((animation) => animation.canceled).length, 4);
  page.move({ turnId: "next-turn", moveSequence: 2 });
  assert.equal(page.elements("browser-agent-click").length, 0);
  page.click();
  assert.equal(page.elements("browser-agent-click").length, 0);
  page.hide();
  assert.equal(page.timers.size, 0);
});

test("hidden state removes feedback and cancels animations immediately", () => {
  const page = createPage();
  const target = page.add("input");
  page.hit(target);
  page.move();
  page.click();
  page.hide();
  assert.equal(page.elements("browser-agent-target")[0].style.display, "none");
  assert.equal(page.elements("browser-agent-click").length, 0);
  assert.equal(page.elements("browser-agent-cursor")[0].style.opacity, "0");
  assert.equal(page.timers.size, 0);
  assert.equal(page.frames.size, 0);
  assert.equal(page.animations[0].canceled, true);
  page.click();
  assert.equal(page.elements("browser-agent-click").length, 0);
});

test("scroll, resize, and pagehide never leave stale markers or click timers", () => {
  for (const event of ["scroll", "resize", "pagehide"]) {
    const page = createPage();
    page.hit(page.add("button"));
    page.move({ animateMovement: false });
    page.click();
    page.emit(event);
    assert.equal(page.elements("browser-agent-click").length, 0, event);
    assert.equal(page.elements("browser-agent-target")[0]?.style.display ?? "none", "none", event);
    page.hide();
    assert.equal(page.timers.size, 0, event);
    assert.equal(page.frames.size, 0, event);
  }
});

test("wrong-session, old-turn, non-press, and invalid point events create no markers", () => {
  const page = createPage();
  page.move();
  for (const override of [
    { sessionId: "other" }, { turnId: "old" }, { eventType: "mouseReleased" },
    { x: NaN }, { x: Infinity }, { x: -1 }, { y: 800 }, { x: "40" }
  ]) page.click(override);
  assert.equal(page.elements("browser-agent-click").length, 0);
  page.hide();
});

test("a late initial state response cannot revive a finalized overlay", async () => {
  const page = createPage();
  page.move();
  page.hide();
  page.initialState({ isVisible: true, sessionId: "test-session", cursor: { x: 40, y: 30 } });
  await Promise.resolve();
  assert.equal(page.elements("browser-agent-cursor")[0].style.opacity, "0");
  assert.equal(page.timers.size, 0);
  page.click();
  assert.equal(page.elements("browser-agent-click").length, 0);
});

function documentStyle(page, id) {
  return page.document.getElementById(id).style;
}
