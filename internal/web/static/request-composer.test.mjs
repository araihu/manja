import test from 'node:test';
import assert from 'node:assert/strict';
import composer from './request-composer.js';

const { buildCurl, composeURL, hydrateRequestComposers } = composer;

test('builds cURL from server variables, path, query, header, and body', () => {
  const curl = buildCurl({
    method: 'post',
    urlTemplate: '{protocol}://{hostname}/api/v3/repos/{owner}/{repo}/hooks',
    serverVariables: {
      protocol: 'https',
      hostname: 'github.example.test',
    },
    parameters: [
      { name: 'owner', in: 'path', value: 'araihu' },
      { name: 'repo', in: 'path', value: 'manja docs' },
      { name: 'per_page', in: 'query', value: '30' },
      { name: 'accept', in: 'header', value: 'application/vnd.github+json' },
    ],
    body: '{\n  "name": "web"\n}',
    bodyContentType: 'application/json',
  });

  assert.equal(curl, [
    'curl --request POST',
    "  --url 'https://github.example.test/api/v3/repos/araihu/manja%20docs/hooks?per_page=30'",
    "  --header 'accept: application/vnd.github+json'",
    "  --header 'content-type: application/json'",
    "  --data '{\n  \"name\": \"web\"\n}'",
  ].join(' \\\n'));
});

test('leaves empty template path parameters in the URL sample', () => {
  assert.equal(composeURL({
    urlTemplate: 'https://api.example.test/repos/{owner}/{repo}',
    parameters: [{ name: 'owner', in: 'path', value: 'araihu' }],
  }), 'https://api.example.test/repos/araihu/{repo}');
});

test('hydrates composer from fields and updates the request sample', () => {
  const root = fakeRoot({
    method: 'GET',
    urlTemplate: '{protocol}://{hostname}/items',
    parameters: [
      { name: 'page', in: 'query', fieldName: 'parameters.page' },
      { name: 'accept', in: 'header', fieldName: 'parameters.accept' },
    ],
  });

  hydrateRequestComposers({ roots: [root] });

  assert.equal(root.code.textContent, [
    'curl --request GET',
    "  --url 'https://api.example.test/items?page=1'",
    "  --header 'accept: application/json'",
  ].join(' \\\n'));

  root.inputs['parameters.page'].value = '2';
  root.inputs['parameters.page'].dispatch('input');

  assert.equal(root.code.textContent, [
    'curl --request GET',
    "  --url 'https://api.example.test/items?page=2'",
    "  --header 'accept: application/json'",
  ].join(' \\\n'));
});

test('samples request body schema before rendering the cURL sample', () => {
  const root = fakeRoot({
    method: 'POST',
    urlTemplate: 'https://api.example.test/hooks',
    parameters: [],
    bodyContentType: 'application/json',
    body: {
      schema: {
        type: 'object',
        properties: { name: { type: 'string' } },
      },
      options: {},
    },
  }, { body: 'Example unavailable' });

  hydrateRequestComposers({
    roots: [root],
    sampler: {
      sample() {
        return { name: 'web' };
      },
    },
  });

  assert.match(root.code.textContent, /--data '{\n  "name": "web"\n}'/);
  assert.equal(root.body.value, '{\n  "name": "web"\n}');
});

function fakeRoot(payload, options = {}) {
  const inputs = {
    'server.protocol': fakeInput('server.protocol', 'https'),
    'server.hostname': fakeInput('server.hostname', 'api.example.test'),
    'parameters.page': fakeInput('parameters.page', '1'),
    'parameters.accept': fakeInput('parameters.accept', 'application/json'),
  };
  const inputList = Object.values(inputs);
  const body = options.body === undefined ? null : fakeTextarea(options.body);
  if (body) {
    inputList.push(body);
  }
  const code = { textContent: '' };
  const payloadScript = { textContent: JSON.stringify(payload) };
  const status = { textContent: '' };

  return {
    dataset: {},
    inputs,
    body,
    code,
    querySelector(selector) {
      if (selector === 'script[id$="-request-composer-payload"][type="application/json"]') return payloadScript;
      if (selector === 'script[type="application/json"]') return payloadScript;
      if (selector === '[data-manja-request-sample] .codeblock code') return code;
      if (selector === '[data-manja-request-body-input]') return body;
      if (selector === '[data-manja-request-body-status]') return status;
      const name = selector.match(/^\[name="(.+)"\]$/)?.[1];
      if (name) return inputs[name] || null;
      throw new Error(`unexpected selector: ${selector}`);
    },
    querySelectorAll(selector) {
      if (selector === '[name^="server."]') {
        return inputList.filter((input) => input.name.startsWith('server.'));
      }
      if (selector === 'input, select, textarea') {
        return inputList;
      }
      throw new Error(`unexpected selector: ${selector}`);
    },
  };
}

function fakeInput(name, value) {
  const listeners = {};
  return {
    name,
    value,
    type: 'text',
    addEventListener(event, handler) {
      listeners[event] = handler;
    },
    dispatch(event) {
      listeners[event]?.();
    },
  };
}

function fakeTextarea(value) {
  return {
    name: 'body',
    value,
    type: 'textarea',
    addEventListener() {},
  };
}
