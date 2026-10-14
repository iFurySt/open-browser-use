import { EventEmitter } from 'node:events';
import { endianness } from 'node:os';
import type { Duplex } from 'node:stream';

export type Message = { jsonrpc: '2.0'; id?: string | number; method?: string; params?: Record<string, any>; result?: any; error?: { code: number; message: string } };
export function frame(message: Message): Buffer {
  const json = Buffer.from(JSON.stringify(message));
  const out = Buffer.alloc(4 + json.length);
  if (endianness() === 'LE') out.writeUInt32LE(json.length); else out.writeUInt32BE(json.length);
  json.copy(out, 4); return out;
}
export class FramedChannel extends EventEmitter {
  private buffer: Buffer = Buffer.alloc(0);
  maxFrame = 4096;
  constructor(readonly stream: Duplex) {
    super();
    stream.on('error', () => stream.destroy());
    stream.on('close', () => this.emit('closed'));
    stream.on('end', () => stream.destroy());
    stream.on('data', (chunk: Buffer) => {
      try {
        this.buffer = Buffer.concat([this.buffer, chunk]);
        while (this.buffer.length >= 4) {
          const size = endianness() === 'LE' ? this.buffer.readUInt32LE() : this.buffer.readUInt32BE();
          if (!size || size > this.maxFrame) throw new Error('Invalid frame size');
          if (this.buffer.length < size + 4) break;
          const message = JSON.parse(this.buffer.subarray(4, size + 4).toString('utf8'));
          this.buffer = this.buffer.subarray(size + 4);
          if (!message || message.jsonrpc !== '2.0' || Array.isArray(message)) throw new Error('Invalid RPC');
          this.emit('message', message);
          if (stream.destroyed) break;
        }
      } catch { stream.destroy(); }
    });
  }
  send(message: Message): void {
    if (this.stream.destroyed) return;
    const data = frame(message);
    if (data.length > this.maxFrame + 4 || this.stream.writableLength > 8 * 1024 * 1024) {
      this.stream.destroy(); return;
    }
    this.stream.write(data);
  }
  close(): void { this.stream.destroy(); }
}
