import { createConnection } from "node:net";
import { endianness } from "node:os";
const headerBytes = 4;
const defaultNavigationTimeoutMs = 10_000;
export class OpenBrowserUseClient {
    socketPath;
    sessionId;
    turnId;
    timeoutMs;
    #socket = null;
    #pendingData = Buffer.alloc(0);
    #nextId = 1;
    #pending = new Map();
    #notificationHandlers = new Set();
    constructor(options) {
        this.socketPath = options.socketPath;
        this.sessionId = options.sessionId ?? "open-browser-use-js";
        this.turnId = options.turnId ?? `turn-${Date.now()}`;
        this.timeoutMs = options.timeoutMs ?? 10_000;
    }
    async connect() {
        if (this.#socket) {
            return this;
        }
        const socket = createConnection(this.socketPath);
        this.#socket = socket;
        socket.on("data", (chunk) => this.#handleData(Buffer.from(chunk)));
        socket.on("close", () => this.#rejectAll(new Error("Open Browser Use socket closed")));
        socket.on("error", (error) => this.#rejectAll(error));
        await new Promise((resolve, reject) => {
            socket.once("connect", resolve);
            socket.once("error", reject);
        });
        return this;
    }
    close() {
        this.#socket?.end();
        this.#socket = null;
    }
    onNotification(handler) {
        this.#notificationHandlers.add(handler);
        return () => {
            this.#notificationHandlers.delete(handler);
        };
    }
    async request(method, params = {}) {
        await this.connect();
        const socket = this.#socket;
        if (!socket) {
            throw new Error("Open Browser Use socket is not connected");
        }
        const id = String(this.#nextId++);
        const suppliedOperationId = params.operation_id;
        const operationId = typeof suppliedOperationId === "string" && suppliedOperationId
            ? suppliedOperationId
            : `op-js-${process.pid}-${Date.now()}-${id}`;
        const mergedParams = {
            session_id: this.sessionId,
            turn_id: this.turnId,
            operation_id: operationId,
            ...params
        };
        const message = {
            jsonrpc: "2.0",
            id,
            method,
            params: mergedParams
        };
        const payload = encodeFrame(message);
        const promise = new Promise((resolve, reject) => {
            const timeout = setTimeout(() => {
                this.#pending.delete(id);
                reject(new Error(`Open Browser Use request timed out: ${method} (operation_id=${operationId}, outcome=unknown; call reconcileOperation)`));
            }, this.timeoutMs);
            this.#pending.set(id, { resolve, reject, timeout, operationId });
        });
        socket.write(payload);
        return await promise;
    }
    getInfo() {
        return this.request("getInfo");
    }
    createTab() {
        return this.request("createTab");
    }
    getTabs() {
        return this.request("getTabs");
    }
    getUserTabs() {
        return this.request("getUserTabs");
    }
    getUserHistory(params = {}) {
        return this.request("getUserHistory", params);
    }
    claimUserTab(tabId) {
        return this.request("claimUserTab", { tabId });
    }
    claimStatus(tabId) {
        return this.request("claimStatus", { tabId });
    }
    closeTab(tabId) {
        return this.request("closeTab", { tabId });
    }
    focusState() {
        return this.request("focusState");
    }
    reconcileOperation(operationId) {
        return this.request("reconcileOperation", { operation_id: operationId });
    }
    finalizeTabs(keep) {
        return this.request("finalizeTabs", { keep });
    }
    nameSession(name) {
        return this.request("nameSession", { name });
    }
    attach(tabId) {
        return this.request("attach", { tabId });
    }
    detach(tabId) {
        return this.request("detach", { tabId });
    }
    executeCdp(tabId, method, commandParams = {}) {
        return this.request("executeCdp", {
            target: { tabId },
            method,
            commandParams
        });
    }
    moveMouse(tabId, x, y, waitForArrival = true) {
        return this.request("moveMouse", { tabId, x, y, waitForArrival });
    }
    waitForFileChooser(tabId, timeoutMs) {
        return this.request("waitForFileChooser", {
            tabId,
            ...(timeoutMs === undefined ? {} : { timeoutMs })
        });
    }
    setFileChooserFiles(fileChooserId, files) {
        return this.request("setFileChooserFiles", { fileChooserId, files });
    }
    waitForDownload(tabId, timeoutMs) {
        return this.request("waitForDownload", {
            tabId,
            ...(timeoutMs === undefined ? {} : { timeoutMs })
        });
    }
    downloadPath(downloadId, timeoutMs) {
        return this.request("downloadPath", {
            downloadId,
            ...(timeoutMs === undefined ? {} : { timeoutMs })
        });
    }
    browserUserHistory(params = {}) {
        return this.getUserHistory(params);
    }
    readClipboardText(tabId) {
        return this.request("readClipboardText", { tabId });
    }
    writeClipboardText(tabId, text) {
        return this.request("writeClipboardText", { tabId, text });
    }
    readClipboard(tabId) {
        return this.request("readClipboard", { tabId });
    }
    writeClipboard(tabId, items) {
        return this.request("writeClipboard", { tabId, items });
    }
    turnEnded() {
        return this.request("turnEnded");
    }
    #handleData(chunk) {
        this.#pendingData = Buffer.concat([this.#pendingData, chunk]);
        while (this.#pendingData.length >= headerBytes) {
            const length = endianness() === "LE"
                ? this.#pendingData.readUInt32LE(0)
                : this.#pendingData.readUInt32BE(0);
            const total = headerBytes + length;
            if (this.#pendingData.length < total) {
                return;
            }
            const payload = this.#pendingData.subarray(headerBytes, total);
            this.#pendingData = this.#pendingData.subarray(total);
            this.#handleMessage(JSON.parse(payload.toString("utf8")));
        }
    }
    #handleMessage(message) {
        if (!isObject(message)) {
            return;
        }
        const id = typeof message.id === "string" || typeof message.id === "number" ? String(message.id) : null;
        if (!id && typeof message.method === "string") {
            const notification = {
                method: message.method,
                params: message.params
            };
            for (const handler of this.#notificationHandlers) {
                handler(notification);
            }
            return;
        }
        if (!id) {
            return;
        }
        const pending = this.#pending.get(id);
        if (!pending) {
            return;
        }
        this.#pending.delete(id);
        clearTimeout(pending.timeout);
        if (isObject(message.error)) {
            pending.reject(new Error(String(message.error.message ?? "Open Browser Use request failed")));
            return;
        }
        pending.resolve((message.result ?? null));
    }
    #rejectAll(error) {
        for (const [id, pending] of this.#pending) {
            this.#pending.delete(id);
            clearTimeout(pending.timeout);
            pending.reject(error);
        }
    }
}
export async function connectOpenBrowserUse(options) {
    const browser = new OpenBrowserUseBrowser(options);
    await browser.connect();
    return browser;
}
export class OpenBrowserUseBrowser {
    client;
    cdp;
    constructor(options) {
        this.client = "client" in options ? options.client : new OpenBrowserUseClient(options);
        this.cdp = new OpenBrowserUseCdp(this.client);
    }
    async connect() {
        await this.client.connect();
        return this;
    }
    close() {
        this.client.close();
    }
    async newTab(options = {}) {
        const created = await this.client.createTab();
        const tabId = tabIdFromValue(created, "createTab response");
        const tab = this.tab(tabId);
        if (options.url) {
            await tab.goto(options.url, options);
        }
        return tab;
    }
    tab(tabId) {
        return new OpenBrowserUseTab(this, tabId);
    }
    getTabs() {
        return this.client.getTabs();
    }
}
export class OpenBrowserUseTab {
    browser;
    id;
    playwright;
    constructor(browser, id) {
        this.browser = browser;
        this.id = id;
        this.playwright = new OpenBrowserUseTabPlaywright(this);
    }
    goto(url, options = {}) {
        return this.browser.cdp.navigate(this.id, url, options);
    }
    waitForLoadState(options = {}) {
        return this.browser.cdp.waitForLoadState(this.id, options);
    }
    domSnapshot() {
        return this.browser.cdp.evaluate(this.id, "document.body?.innerText ?? ''").then((value) => String(value ?? ""));
    }
    evaluate(expression, options = {}) {
        return this.browser.cdp.evaluate(this.id, expression, options);
    }
    close() {
        return this.browser.cdp.call(this.id, "Page.close");
    }
}
export class OpenBrowserUseTabPlaywright {
    tab;
    constructor(tab) {
        this.tab = tab;
    }
    waitForLoadState(options = {}) {
        return this.tab.waitForLoadState(options);
    }
    domSnapshot() {
        return this.tab.domSnapshot();
    }
}
export class OpenBrowserUseCdp {
    client;
    #attachedTabIds = new Set();
    constructor(client) {
        this.client = client;
    }
    async call(tabId, method, commandParams = {}, options = {}) {
        await this.ensureAttached(tabId);
        return this.client.request("executeCdp", {
            target: { tabId },
            method,
            commandParams,
            ...(options.timeoutMs === undefined ? {} : { timeoutMs: options.timeoutMs })
        });
    }
    async evaluate(tabId, expression, options = {}) {
        const result = await this.call(tabId, "Runtime.evaluate", {
            expression,
            returnByValue: true,
            ...(options.awaitPromise === undefined ? {} : { awaitPromise: options.awaitPromise })
        });
        if (isObject(result) && isObject(result.exceptionDetails)) {
            throw new Error(String(result.exceptionDetails.text ?? "Open Browser Use evaluation failed"));
        }
        return isObject(result) && isObject(result.result) ? (result.result.value ?? null) : null;
    }
    async navigate(tabId, url, options = {}) {
        if (!url) {
            throw new Error("goto requires a URL");
        }
        const waitUntil = options.waitUntil ?? "load";
        assertSupportedLoadState(waitUntil);
        const timeoutMs = options.timeoutMs ?? defaultNavigationTimeoutMs;
        await this.call(tabId, "Page.enable");
        const wait = this.waitForLoadEvent(tabId, waitUntil, timeoutMs);
        const result = await this.call(tabId, "Page.navigate", { url }, { timeoutMs });
        if (isObject(result) && typeof result.errorText === "string" && result.errorText) {
            throw new Error(`Browser failed to navigate tab ${tabId}: ${result.errorText}`);
        }
        await wait.catch(async (error) => {
            const state = await this.readDocumentState(tabId);
            if (documentStateMatches(state, waitUntil)) {
                return;
            }
            throw error;
        });
        return result;
    }
    async waitForLoadState(tabId, options = {}) {
        const state = options.state ?? "load";
        assertSupportedLoadState(state);
        await this.call(tabId, "Page.enable");
        const documentState = await this.readDocumentState(tabId);
        if (documentStateMatches(documentState, state)) {
            return;
        }
        await this.waitForLoadEvent(tabId, state, options.timeoutMs ?? defaultNavigationTimeoutMs);
    }
    async readDocumentState(tabId) {
        try {
            const value = await this.evaluate(tabId, "({ href: window.location.href, readyState: document.readyState })");
            return isObject(value) ? { href: stringValue(value.href), readyState: stringValue(value.readyState) } : undefined;
        }
        catch {
            return undefined;
        }
    }
    waitForEvent(tabId, predicate, options = {}) {
        const timeoutMs = options.timeoutMs ?? defaultNavigationTimeoutMs;
        return new Promise((resolve, reject) => {
            let settled = false;
            let removeHandler = null;
            let timer;
            const finish = () => {
                if (settled) {
                    return false;
                }
                settled = true;
                clearTimeout(timer);
                removeHandler?.();
                return true;
            };
            timer = setTimeout(() => {
                if (finish()) {
                    reject(new Error(options.timeoutMessage ?? `Timed out waiting for tab ${tabId} event`));
                }
            }, timeoutMs);
            removeHandler = this.client.onNotification((notification) => {
                try {
                    if (predicate(notification)) {
                        if (finish()) {
                            resolve(notification);
                        }
                    }
                }
                catch (error) {
                    if (finish()) {
                        reject(error instanceof Error ? error : new Error(String(error)));
                    }
                }
            });
        });
    }
    waitForLoadEvent(tabId, state, timeoutMs) {
        return this.waitForEvent(tabId, (notification) => {
            const event = cdpEventForTab(notification, tabId);
            if (!event) {
                return false;
            }
            if (event.method === "Page.navigationBlocked") {
                throw new Error(`Navigation was blocked in tab ${tabId}`);
            }
            return state === "domcontentloaded"
                ? event.method === "Page.domContentEventFired"
                : event.method === "Page.loadEventFired";
        }, {
            timeoutMs,
            timeoutMessage: `Timed out waiting for ${state} in tab ${tabId}`
        }).then(() => undefined);
    }
    async ensureAttached(tabId) {
        if (this.#attachedTabIds.has(tabId)) {
            return;
        }
        await this.client.attach(tabId);
        this.#attachedTabIds.add(tabId);
    }
}
export function encodeFrame(value) {
    const payload = Buffer.from(JSON.stringify(value), "utf8");
    const frame = Buffer.alloc(headerBytes + payload.length);
    if (endianness() === "LE") {
        frame.writeUInt32LE(payload.length, 0);
    }
    else {
        frame.writeUInt32BE(payload.length, 0);
    }
    payload.copy(frame, headerBytes);
    return frame;
}
function isObject(value) {
    return typeof value === "object" && value !== null && !Array.isArray(value);
}
function tabIdFromValue(value, label) {
    if (!isObject(value)) {
        throw new Error(`${label} did not include a tab object`);
    }
    const id = value.id;
    if (typeof id === "number" && Number.isInteger(id) && id > 0) {
        return id;
    }
    if (typeof id === "string") {
        const parsed = Number(id);
        if (Number.isInteger(parsed) && parsed > 0) {
            return parsed;
        }
    }
    throw new Error(`${label} did not include a numeric tab id`);
}
function assertSupportedLoadState(state) {
    if (state !== "domcontentloaded" && state !== "load") {
        throw new Error(`Unsupported load state "${state}". Use "domcontentloaded" or "load".`);
    }
}
function documentStateMatches(documentState, state) {
    if (documentState?.readyState === "complete") {
        return true;
    }
    return state === "domcontentloaded" && documentState?.readyState === "interactive";
}
function cdpEventForTab(notification, tabId) {
    if (notification.method !== "onCDPEvent" || !isObject(notification.params)) {
        return null;
    }
    const source = notification.params.source;
    if (!isObject(source) || source.tabId !== tabId) {
        return null;
    }
    return {
        method: stringValue(notification.params.method)
    };
}
function stringValue(value) {
    return typeof value === "string" ? value : undefined;
}
