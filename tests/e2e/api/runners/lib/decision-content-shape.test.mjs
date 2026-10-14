import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';

const collection = JSON.parse(fs.readFileSync(new URL('../../collections/provider-harness.json', import.meta.url), 'utf8'));
const collectionScript = collection.event.find(event => event.listen === 'test').script.exec.join('\n');
const contentScript = collectionScript.slice(
  collectionScript.indexOf('// Only register the content-shape test'),
  collectionScript.indexOf('// Routed-identity response headers'),
);

function contentFailures(body, { status = 200, method = '/v1/decisions', rawBody = '', rawResponse = JSON.stringify(body) } = {}) {
  const failures = [];
  new Function('pm', contentScript)({
    request: { url: { toString: () => `http://localhost:8080${method}` }, body: { raw: rawBody } },
    response: {
      code: status,
      headers: { get: () => 'application/json' },
      json: () => body,
      text: () => rawResponse,
    },
    test: (_name, fn) => { try { fn(); } catch (error) { failures.push(error); } },
    expect: (value) => ({ to: { be: { get true() { assert.equal(value, true); } } } }),
  });
  return failures;
}

test('decision answers count as response content', () => {
  assert.equal(contentFailures({ answers: { category: { kind: 'choice', value: 'billing' } } }).length, 0);
});

test('empty decision answers still fail the content check', () => {
  assert.equal(contentFailures({ answers: {} }).length, 1);
});

test('valid MCP JSON-RPC result and error envelopes count as protocol content', () => {
  assert.equal(contentFailures({ jsonrpc: '2.0', id: 1, result: { tools: [] } }, { method: '/mcp' }).length, 0);
  assert.equal(contentFailures({ jsonrpc: '2.0', id: 1, error: { code: -32602, message: 'invalid params' } }, { method: '/mcp' }).length, 0);
});

test('JSON-RPC envelopes do not bypass content checks on non-MCP routes', () => {
  const jsonrpc = { jsonrpc: '2.0', id: 1, result: { tools: [] } };
  assert.equal(contentFailures(jsonrpc, { method: '/v1/chat/completions' }).length, 1);
  assert.equal(contentFailures(jsonrpc, { method: '/mcpapps' }).length, 1, 'route name must match the MCP path segment');
});

test('invalid MCP JSON-RPC envelopes do not pass the shape check', () => {
  assert.equal(contentFailures({ jsonrpc: '2.0', result: { tools: [] } }, { method: '/mcp' }).length, 1, 'response ID is required');
  assert.equal(contentFailures({ jsonrpc: '2.0', id: 1, result: {}, error: { code: -1, message: 'bad' } }, { method: '/mcp' }).length, 1, 'result and error are mutually exclusive');
  assert.equal(contentFailures({ jsonrpc: '2.0', id: 1, error: { code: 'bad', message: 'invalid params' } }, { method: '/mcp' }).length, 1, 'error code must be numeric');
});

test('accepted empty MCP notifications use 202 or 204 without content validation', () => {
  const notification = JSON.stringify({ jsonrpc: '2.0', method: 'notifications/initialized' });
  for (const status of [202, 204]) {
    assert.equal(contentFailures(undefined, { status, method: '/mcp/client', rawBody: notification, rawResponse: '' }).length, 0, `HTTP ${status}`);
  }
  assert.equal(contentFailures(undefined, { status: 200, method: '/mcp/client', rawBody: notification, rawResponse: '' }).length, 1, 'unexpected 200 empty body must still fail');
});
