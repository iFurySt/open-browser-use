(() => {
  const STATE_KEY = Symbol.for("open-browser-use.webmcp.main");
  const REQUEST_SOURCE = "open-browser-use-webmcp-bridge";
  const RESPONSE_SOURCE = "open-browser-use-webmcp-page";
  const DEFAULT_INVOKE_TIMEOUT_MS = 10_000;

  if (globalThis[STATE_KEY]) {
    return;
  }

  const state = {
    modelContext: null,
    nextRegistrationId: 1,
    registrations: new Map(),
    toolChangeListener: null
  };
  Object.defineProperty(globalThis, STATE_KEY, {
    configurable: false,
    enumerable: false,
    value: state,
    writable: false
  });

  ensureModelContext();

  window.addEventListener("message", (event) => {
    if (event.source !== window || !isBridgeRequest(event.data)) {
      return;
    }
    const requestId = event.data.requestId;
    void handleRequest(event.data)
      .then((result) => {
        window.postMessage({ source: RESPONSE_SOURCE, requestId, ok: true, result }, "*");
      })
      .catch((error) => {
        window.postMessage(
          {
            source: RESPONSE_SOURCE,
            requestId,
            ok: false,
            error: error instanceof Error ? error.message : String(error)
          },
          "*"
        );
      });
  });

  async function handleRequest(request) {
    switch (request.action) {
      case "list":
        return await listTools();
      case "invoke":
        return await invokeTool(request);
      default:
        throw new Error(`Unsupported WebMCP bridge action: ${String(request.action)}`);
    }
  }

  async function listTools() {
    const modelContext = document.modelContext;
    observeModelContext(modelContext);
    state.registrations.clear();
    if (!modelContext || typeof modelContext.getTools !== "function") {
      return { tools: [] };
    }

    const registeredTools = await modelContext.getTools();
    if (!Array.isArray(registeredTools)) {
      throw new Error("document.modelContext.getTools() did not return an array");
    }

    const tools = [];
    for (const registeredTool of registeredTools) {
      if (!registeredTool || typeof registeredTool.name !== "string" || !registeredTool.name.trim()) {
        continue;
      }
      const registrationId = createRegistrationId();
      state.registrations.set(registrationId, {
        modelContext,
        name: registeredTool.name,
        tool: registeredTool
      });
      tools.push(toWireTool(registeredTool, registrationId));
    }
    return { tools };
  }

  async function invokeTool(request) {
    if (typeof request.registration_id !== "string" || !request.registration_id) {
      throw new Error("WebMCP invocation requires registration_id");
    }
    if (typeof request.tool_name !== "string" || !request.tool_name.trim()) {
      throw new Error("WebMCP invocation requires tool_name");
    }
    const registration = state.registrations.get(request.registration_id);
    if (!registration || registration.name !== request.tool_name) {
      throw new Error("WebMCP tool registration is stale; list tools again");
    }

    const modelContext = document.modelContext;
    if (modelContext !== registration.modelContext || typeof modelContext?.executeTool !== "function") {
      state.registrations.delete(request.registration_id);
      throw new Error("WebMCP tool registration is stale; list tools again");
    }

    const timeoutMs = positiveTimeout(request.timeout_ms, DEFAULT_INVOKE_TIMEOUT_MS);
    const controller = new AbortController();
    let timeoutId;
    try {
      const inputJson = JSON.stringify(request.input);
      if (inputJson === undefined) {
        throw new Error("WebMCP tool input must be JSON serializable");
      }
      const rawResult = await Promise.race([
        Promise.resolve(
          modelContext.executeTool(registration.tool, inputJson, { signal: controller.signal })
        ),
        new Promise((_, reject) => {
          timeoutId = setTimeout(() => {
            controller.abort();
            reject(new Error(`WebMCP tool invocation timed out after ${timeoutMs}ms`));
          }, timeoutMs);
        })
      ]);
      return { result: normalizeToolResult(rawResult) };
    } finally {
      clearTimeout(timeoutId);
    }
  }

  function ensureModelContext() {
    if (document.modelContext) {
      return document.modelContext;
    }

    const listeners = new Set();
    const registrations = new Map();
    const descriptorRecords = new WeakMap();
    const modelContext = {
      async registerTool(tool, options = {}) {
        const normalized = normalizeShimTool(tool);
        if (registrations.has(normalized.name)) {
          throw namedError("InvalidStateError", `WebMCP tool ${JSON.stringify(normalized.name)} is already registered`);
        }
        if (options?.signal?.aborted) {
          throw namedError("AbortError", "WebMCP tool registration was aborted");
        }
        const record = {
          name: normalized.name,
          tool: normalized,
          signal: options?.signal ?? null,
          abortListener: null
        };
        if (record.signal && typeof record.signal.addEventListener === "function") {
          record.abortListener = () => {
            if (registrations.get(record.name) !== record) {
              return;
            }
            registrations.delete(record.name);
            emitToolChange();
          };
          record.signal.addEventListener("abort", record.abortListener, { once: true });
        }
        registrations.set(record.name, record);
        emitToolChange();
      },

      async getTools() {
        return [...registrations.values()]
          .sort((left, right) => left.name.localeCompare(right.name))
          .map((record) => {
            const descriptor = {
              name: record.tool.name,
              ...(record.tool.title ? { title: record.tool.title } : {}),
              description: record.tool.description,
              inputSchema: JSON.stringify(record.tool.inputSchema),
              ...(record.tool.annotations ? { annotations: record.tool.annotations } : {}),
              origin: location.origin,
              window
            };
            descriptorRecords.set(descriptor, record);
            return descriptor;
          });
      },

      async executeTool(descriptor, input, options = {}) {
        const record = descriptorRecords.get(descriptor);
        if (!record || registrations.get(record.name) !== record) {
          throw namedError("InvalidStateError", "WebMCP tool registration is no longer active");
        }
        const args = typeof input === "string" ? JSON.parse(input) : input;
        return await record.tool.execute(args, { signal: options?.signal });
      },

      addEventListener(type, listener) {
        if (type === "toolchange" && typeof listener === "function") {
          listeners.add(listener);
        }
      },

      removeEventListener(type, listener) {
        if (type === "toolchange") {
          listeners.delete(listener);
        }
      }
    };

    function emitToolChange() {
      for (const listener of listeners) {
        try {
          listener({ type: "toolchange", target: modelContext });
        } catch {}
      }
    }

    try {
      Object.defineProperty(document, "modelContext", {
        configurable: true,
        enumerable: true,
        value: modelContext,
        writable: false
      });
    } catch {
      return document.modelContext ?? null;
    }
    return modelContext;
  }

  function normalizeShimTool(tool) {
    if (!tool || typeof tool !== "object") {
      throw new TypeError("WebMCP registerTool requires a tool object");
    }
    const name = typeof tool.name === "string" ? tool.name.trim() : "";
    const description = typeof tool.description === "string" ? tool.description.trim() : "";
    if (!name || !description || typeof tool.execute !== "function") {
      throw namedError("InvalidStateError", "WebMCP tool requires name, description, and execute");
    }
    const inputSchema = tool.inputSchema === undefined ? {} : toJsonValue(tool.inputSchema);
    const annotations = tool.annotations === undefined ? undefined : toJsonValue(tool.annotations);
    return {
      name,
      ...(typeof tool.title === "string" && tool.title.trim() ? { title: tool.title.trim() } : {}),
      description,
      inputSchema,
      ...(annotations && typeof annotations === "object" ? { annotations } : {}),
      execute: tool.execute
    };
  }

  function namedError(name, message) {
    const error = new Error(message);
    error.name = name;
    return error;
  }

  function observeModelContext(modelContext) {
    if (state.modelContext === modelContext) {
      return;
    }
    if (
      state.modelContext &&
      state.toolChangeListener &&
      typeof state.modelContext.removeEventListener === "function"
    ) {
      state.modelContext.removeEventListener("toolchange", state.toolChangeListener);
    }
    state.modelContext = modelContext ?? null;
    state.registrations.clear();
    state.toolChangeListener = null;
    if (modelContext && typeof modelContext.addEventListener === "function") {
      const listener = () => {
        state.registrations.clear();
      };
      modelContext.addEventListener("toolchange", listener);
      state.toolChangeListener = listener;
    }
  }

  function toWireTool(tool, registrationId) {
    const annotations = normalizeAnnotations(tool.annotations);
    const fallbackOrigin = typeof location?.origin === "string" && location.origin !== "null"
      ? location.origin
      : undefined;
    return {
      name: tool.name,
      registration_id: registrationId,
      ...(typeof tool.title === "string" ? { title: tool.title } : {}),
      ...(typeof tool.description === "string" ? { description: tool.description } : {}),
      input_schema: normalizeInputSchema(tool.inputSchema),
      ...(annotations ? { annotations } : {}),
      ...(typeof tool.origin === "string"
        ? { origin: tool.origin }
        : fallbackOrigin
          ? { origin: fallbackOrigin }
          : {}),
      ...(typeof location?.href === "string" ? { pageUrl: location.href } : {})
    };
  }

  function normalizeInputSchema(inputSchema) {
    if (typeof inputSchema === "string") {
      try {
        return toJsonValue(JSON.parse(inputSchema));
      } catch {
        return inputSchema;
      }
    }
    return inputSchema === undefined ? {} : toJsonValue(inputSchema);
  }

  function normalizeAnnotations(annotations) {
    if (!annotations || typeof annotations !== "object") {
      return null;
    }
    const normalized = {};
    if (typeof annotations.readOnlyHint === "boolean") {
      normalized.readOnlyHint = annotations.readOnlyHint;
    }
    if (typeof annotations.untrustedContentHint === "boolean") {
      normalized.untrustedContentHint = annotations.untrustedContentHint;
    }
    return Object.keys(normalized).length > 0 ? normalized : null;
  }

  function normalizeToolResult(result) {
    if (result === undefined) {
      return null;
    }
    if (typeof result === "string") {
      const trimmed = result.trim();
      if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
        try {
          return toJsonValue(JSON.parse(trimmed));
        } catch {}
      }
      return result;
    }
    return toJsonValue(result);
  }

  function toJsonValue(value) {
    const serialized = JSON.stringify(value);
    if (serialized === undefined) {
      throw new Error("WebMCP value is not JSON serializable");
    }
    return JSON.parse(serialized);
  }

  function createRegistrationId() {
    if (typeof crypto?.randomUUID === "function") {
      return crypto.randomUUID();
    }
    const id = `obu-webmcp-${Date.now()}-${state.nextRegistrationId}`;
    state.nextRegistrationId += 1;
    return id;
  }

  function positiveTimeout(value, fallback) {
    return Number.isInteger(value) && value > 0 ? value : fallback;
  }

  function isBridgeRequest(value) {
    return (
      value &&
      typeof value === "object" &&
      value.source === REQUEST_SOURCE &&
      typeof value.requestId === "string"
    );
  }
})();
