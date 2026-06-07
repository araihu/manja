import sampler from 'openapi-sampler';
import { HTTPSnippet } from '@readme/httpsnippet';

let input = '';
for await (const chunk of process.stdin) {
  input += chunk;
}

const payload = JSON.parse(input || '{}');

if (payload.mode === 'sample') {
  const sample = sampler.sample(payload.schema, { skipNonRequired: false });
  process.stdout.write(JSON.stringify(sample, null, 2));
} else if (payload.mode === 'snippet') {
  const snippet = new HTTPSnippet(payload.request);
  const converted = snippet.convert('shell', 'curl');
  const code = Array.isArray(converted) ? converted[0] : converted;
  process.stdout.write(code.replaceAll('%7B', '{').replaceAll('%7D', '}'));
} else {
  throw new Error(`unknown fragment mode: ${payload.mode}`);
}
