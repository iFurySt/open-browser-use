import { endianness } from "node:os";
import type { Socket } from "node:net";

export type JsonValue =
  | null
  | boolean
  | number
  | string
  | JsonValue[]
  | {
      [key: string]: JsonValue;
    };

export const WEBMCP_LIST_TOOLS_METHOD = "webmcp_list_tools";
export const WEBMCP_INVOKE_TOOL_METHOD = "webmcp_invoke_tool";

export type BrowserSessionParams = {
  session_id: string;
  turn_id: string;
};

export type WebMcpToolAnnotations = {
  readOnlyHint?: boolean;
  untrustedContentHint?: boolean;
};

export type WebMcpToolDescriptor = {
  name: string;
  registration_id: string;
  title?: string;
  description?: string;
  input_schema: JsonValue;
  annotations?: WebMcpToolAnnotations;
  origin?: string;
  pageUrl?: string;
};

export type WebMcpListToolsParams = BrowserSessionParams & {
  tabId: number;
};

export type WebMcpListToolsResult = {
  tools: WebMcpToolDescriptor[];
};

export type WebMcpInvokeToolParams = BrowserSessionParams & {
  tabId: number;
  tool_name: string;
  tool_description?: string;
  tool_title?: string;
  registration_id: string;
  input: JsonValue;
  timeout_ms?: number;
};

export type WebMcpInvokeToolResult = {
  result: JsonValue;
};

export type JsonRpcRequest = {
  jsonrpc?: "2.0";
  id?: number | string | null;
  method: string;
  params?: JsonValue;
};

export type JsonRpcResponse =
  | {
      jsonrpc: "2.0";
      id: number | string | null;
      result: JsonValue;
    }
  | {
      jsonrpc: "2.0";
      id: number | string | null;
      error: {
        code: number;
        message: string;
      };
    };

export type JsonRpcHandler = (params: JsonValue | undefined) => JsonValue | Promise<JsonValue>;

const frameHeaderBytes = 4;

export function encodeFrame(message: JsonValue): Buffer {
  const payload = Buffer.from(JSON.stringify(message), "utf8");
  const frame = Buffer.alloc(frameHeaderBytes + payload.length);
  if (endianness() === "LE") {
    frame.writeUInt32LE(payload.length, 0);
  } else {
    frame.writeUInt32BE(payload.length, 0);
  }
  payload.copy(frame, frameHeaderBytes);
  return frame;
}

export function decodeFrames(buffer: Buffer): {
  messages: JsonValue[];
  remainingData: Buffer;
} {
  const messages: JsonValue[] = [];
  let offset = 0;
  while (buffer.length - offset >= frameHeaderBytes) {
    const length =
      endianness() === "LE" ? buffer.readUInt32LE(offset) : buffer.readUInt32BE(offset);
    const total = frameHeaderBytes + length;
    if (buffer.length - offset < total) {
      break;
    }
    const payload = buffer.subarray(offset + frameHeaderBytes, offset + total).toString("utf8");
    messages.push(JSON.parse(payload) as JsonValue);
    offset += total;
  }
  return {
    messages,
    remainingData: buffer.subarray(offset)
  };
}

export class JsonRpcSocketConnection {
  #pendingData: Buffer<ArrayBufferLike> = Buffer.alloc(0);
  #handlers = new Map<string, JsonRpcHandler>();

  constructor(private readonly socket: Socket) {
    this.socket.on("data", (chunk) => this.#handleData(Buffer.from(chunk)));
  }

  registerHandler(method: string, handler: JsonRpcHandler): void {
    this.#handlers.set(method, handler);
  }

  sendNotification(method: string, params?: JsonValue): void {
    this.socket.write(encodeFrame(params === undefined ? { jsonrpc: "2.0", method } : { jsonrpc: "2.0", method, params }));
  }

  close(): void {
    this.socket.end();
  }

  #handleData(chunk: Buffer): void {
    this.#pendingData = Buffer.from(Buffer.concat([this.#pendingData, chunk]));
    const { messages, remainingData } = decodeFrames(this.#pendingData);
    this.#pendingData = remainingData;
    for (const message of messages) {
      void this.#handleMessage(message);
    }
  }

  async #handleMessage(message: JsonValue): Promise<void> {
    if (!isJsonRpcRequest(message)) {
      return;
    }
    if (message.id === undefined) {
      return;
    }

    const handler = this.#handlers.get(message.method);
    if (!handler) {
      this.#sendResponse({
        jsonrpc: "2.0",
        id: message.id,
        error: {
          code: -32601,
          message: `No handler registered for method: ${message.method}`
        }
      });
      return;
    }

    try {
      this.#sendResponse({
        jsonrpc: "2.0",
        id: message.id,
        result: await handler(message.params)
      });
    } catch (error) {
      this.#sendResponse({
        jsonrpc: "2.0",
        id: message.id,
        error: {
          code: 1,
          message: error instanceof Error ? error.message : String(error)
        }
      });
    }
  }

  #sendResponse(response: JsonRpcResponse): void {
    this.socket.write(encodeFrame(response));
  }
}

export function isJsonObject(value: JsonValue | undefined): value is { [key: string]: JsonValue } {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isJsonRpcRequest(value: JsonValue): value is JsonRpcRequest {
  return (
    isJsonObject(value) &&
    typeof value.method === "string" &&
    (value.id === undefined ||
      typeof value.id === "number" ||
      typeof value.id === "string" ||
      value.id === null)
  );
}
