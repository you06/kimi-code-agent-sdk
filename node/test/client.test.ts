import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { afterEach, describe, expect, it } from 'vitest';

import { connect, ProtocolError } from '../src/index.js';

const tempDirs: string[] = [];

afterEach(async () => {
  for (const dir of tempDirs.splice(0)) {
    await rm(dir, { recursive: true, force: true });
  }
});

describe('Node client', () => {
  it('creates a session and streams prompt events from a stdio sdk server', async () => {
    const server = await writeFakeServer();
    const client = await connect({
      executable: process.execPath,
      args: [server],
      clientName: 'node-client-test',
    });

    try {
      const session = await client.createSession({
        id: 'ses_node_test',
        workDir: '/tmp/project',
        thinking: false,
        permission: 'auto',
      });

      expect(session.id).toBe('ses_node_test');
      expect(session.workDir).toBe('/tmp/project');

      const events = [];
      for await (const event of session.prompt('hello')) {
        events.push(event);
      }

      expect(events).toEqual([
        expect.objectContaining({ type: 'turn.started', turnId: 0 }),
        expect.objectContaining({
          type: 'assistant.delta',
          turnId: 0,
          delta: 'hello from fake server',
        }),
        expect.objectContaining({ type: 'turn.ended', turnId: 0, reason: 'completed' }),
      ]);
    } finally {
      await client.close();
    }
  });

  it('maps protocol errors to ProtocolError', async () => {
    const server = await writeFakeServer();
    const client = await connect({ executable: process.execPath, args: [server] });

    try {
      await expect(client.createSession({ workDir: '/tmp/missing' })).rejects.toMatchObject({
        name: 'ProtocolError',
        code: 'INVALID_INPUT',
      } satisfies Partial<ProtocolError>);
    } finally {
      await client.close();
    }
  });
});

async function writeFakeServer(): Promise<string> {
  const dir = await mkdtemp(join(tmpdir(), 'kimi-code-agent-sdk-node-'));
  tempDirs.push(dir);
  const file = join(dir, 'fake-server.mjs');
  await writeFile(
    file,
    `
import { createInterface } from 'node:readline';

function write(message) {
  process.stdout.write(JSON.stringify(message) + '\\n');
}

for await (const line of createInterface({ input: process.stdin, crlfDelay: Infinity })) {
  if (line.trim().length === 0) continue;
  const request = JSON.parse(line);
  if (request.method === 'initialize') {
    write({
      jsonrpc: '2.0',
      id: request.id,
      result: {
        protocolVersion: '1.0',
        supportedVersions: ['1.0'],
        server: { name: 'kimi-code', version: '0.0.0-test' },
        capabilities: {},
      },
    });
    continue;
  }
  if (request.method === 'createSession') {
    if (request.params.id !== 'ses_node_test') {
      write({
        jsonrpc: '2.0',
        id: request.id,
        error: {
          code: 'INVALID_INPUT',
          message: 'id must be ses_node_test',
          data: { retryable: false },
        },
      });
      continue;
    }
    write({
      jsonrpc: '2.0',
      id: request.id,
      result: {
        id: 'ses_node_test',
        workDir: request.params.workDir,
        sessionDir: '/tmp/session',
        createdAt: 1,
        updatedAt: 2,
      },
    });
    continue;
  }
  if (request.method === 'prompt') {
    write({
      jsonrpc: '2.0',
      method: 'event',
      params: { type: 'turn.started', sessionId: request.params.sessionId, agentId: 'main', turnId: 0 },
    });
    write({
      jsonrpc: '2.0',
      method: 'event',
      params: {
        type: 'assistant.delta',
        sessionId: request.params.sessionId,
        agentId: 'main',
        turnId: 0,
        delta: 'hello from fake server',
      },
    });
    write({
      jsonrpc: '2.0',
      method: 'event',
      params: {
        type: 'turn.ended',
        sessionId: request.params.sessionId,
        agentId: 'main',
        turnId: 0,
        reason: 'completed',
      },
    });
    write({ jsonrpc: '2.0', id: request.id, result: { turnId: 0 } });
    continue;
  }
  if (request.method === 'shutdown') {
    write({ jsonrpc: '2.0', id: request.id, result: {} });
    process.exit(0);
  }
}
`,
    'utf-8',
  );
  return file;
}
