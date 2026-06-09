import { execFile } from 'node:child_process';
import assert from 'node:assert/strict';
import { promisify } from 'node:util';
import test from 'node:test';

const execFileAsync = promisify(execFile);

test('print-ports includes the product site server', async () => {
  const { stdout } = await execFileAsync(
    process.execPath,
    ['scripts/dev-server.mjs', '--print-ports'],
    {
      cwd: new URL('..', import.meta.url),
      env: {
        ...process.env,
        MANJA_DEV_APP_PORT: '18132',
        MANJA_DEV_PROXY_PORT: '17383',
        MANJA_DEV_SITE_PORT: '18133',
      },
    },
  );

  const ports = JSON.parse(stdout);
  assert.equal(ports.appURL, 'http://127.0.0.1:18132');
  assert.equal(ports.proxyURL, 'http://127.0.0.1:17383');
  assert.equal(ports.siteURL, 'http://127.0.0.1:18133');
  assert.equal(ports.sitePort, 18133);
  assert.deepEqual(ports.siteArgs, [
    'run',
    './cmd/server',
    '-addr',
    '127.0.0.1:18133',
  ]);
});
