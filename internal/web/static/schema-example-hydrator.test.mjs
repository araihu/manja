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

test('passes spec context so referenced schemas can be sampled', () => {
  const spec = { components: { schemas: { Todo: objectSchema() } } };
  const root = fakeRoot('ref', {
    schema: { $ref: '#/components/schemas/Todo' },
    spec,
    options: { skipNonRequired: true },
  }, 'fallback');

  hydrateSchemaExamples({
    roots: [root],
    sampler: {
      sample(schema, options, receivedSpec) {
        assert.deepEqual(schema, { $ref: '#/components/schemas/Todo' });
        assert.deepEqual(receivedSpec, spec);
        return sampleSchema(receivedSpec.components.schemas.Todo, options);
      },
    },
  });

  assert.equal(root.code.textContent, '{\n  "name": "string"\n}');
  assert.equal(root.status.textContent, 'Example generated');
});

test('updates the nested code element without replacing the codeblock wrapper', () => {
  const root = fakeRoot('nested', { schema: objectSchema(), options: {} }, 'fallback');

  hydrateSchemaExamples({ roots: [root], sampler: { sample: sampleSchema } });

  assert.equal(root.code.textContent, '{\n  "name": "string",\n  "done": true\n}');
  assert.equal(root.codeblock.textContent, 'fallback');
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
  const codeblock = { textContent: fallback };
  const code = { textContent: fallback };
  const status = { textContent: '' };
  const payloadScript = { textContent: JSON.stringify(payload) };
  return {
    dataset: {},
    codeblock,
    code,
    status,
    payloadScript,
    querySelector(selector) {
      if (selector === '.codeblock code') return code;
      if (selector === '.codeblock') return codeblock;
      if (selector === 'script[type="application/json"]') return payloadScript;
      if (selector === '[data-manja-example-status]') return status;
      throw new Error(`unexpected selector: ${selector}`);
    },
  };
}
