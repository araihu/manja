import test from 'node:test';
import assert from 'node:assert/strict';
import hydrator from './schema-example-hydrator.js';

const { hydrateSchemaExamples } = hydrator;

test('hydrates multiple examples and honors skipNonRequired', () => {
  const roots = [
    fakeRoot('a', { schema: objectSchema(), options: { skipNonRequired: true } }, 'fallback'),
    fakeRoot('b', { schema: objectSchema(), options: { skipNonRequired: false } }, 'fallback'),
  ];

  hydrateSchemaExamples({ roots, sampler: { sample: sampleSchema } });

  assert.equal(roots[0].code.textContent, '{\n  "name": "string"\n}');
  assert.equal(roots[1].code.textContent, '{\n  "name": "string",\n  "done": true\n}');
});

test('preserves fallback when sampling fails and is idempotent', () => {
  const root = fakeRoot('broken', { schema: { type: 'object' }, options: {} }, 'fallback');

  hydrateSchemaExamples({ roots: [root], sampler: { sample() { throw new Error('boom'); } } });
  hydrateSchemaExamples({ roots: [root], sampler: { sample() { throw new Error('boom'); } } });

  assert.equal(root.code.textContent, 'fallback');
  assert.equal(root.status.textContent, 'Example unavailable');
  assert.equal(root.dataset.manjaExampleHydrated, 'true');
});

function objectSchema() {
  return {
    type: 'object',
    required: ['name'],
    properties: {
      name: { type: 'string' },
      done: { type: 'boolean' },
    },
  };
}

function sampleSchema(schema, options = {}) {
  const sample = {};
  for (const [name, property] of Object.entries(schema.properties || {})) {
    if (options.skipNonRequired && !(schema.required || []).includes(name)) {
      continue;
    }
    sample[name] = property.type === 'boolean' ? true : 'string';
  }
  return sample;
}

function fakeRoot(id, payload, fallback) {
  const code = { textContent: fallback };
  const status = { textContent: '' };
  const payloadScript = { textContent: JSON.stringify(payload) };
  return {
    dataset: {},
    code,
    status,
    payloadScript,
    querySelector(selector) {
      if (selector === '.codeblock') return code;
      if (selector === 'script[type="application/json"]') return payloadScript;
      if (selector === '[data-manja-example-status]') return status;
      throw new Error(`unexpected selector: ${selector}`);
    },
  };
}
