import test from 'node:test';
import assert from 'node:assert/strict';
import worker from './index.mjs';

test('legacy links preserve paths, encoding and query parameters', () => {
  for (const path of ['/', '/macos-usage/', '/installation?from=readme&next=%2Fusage', '/a%20b?q=a%2Bb']) {
    const response = worker.fetch(new Request(`https://aims.apps.doguyilmaz.com${path}`));
    assert.equal(response.status, 301);
    assert.equal(response.headers.get('location'), `https://aims.doguyilmaz.com${path}`);
  }
});

test('unrelated hosts are never redirected', () => {
  assert.equal(worker.fetch(new Request('https://unrelated.example/path')).status, 404);
});
