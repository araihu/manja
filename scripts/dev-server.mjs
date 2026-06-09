#!/usr/bin/env node
import { spawn } from 'node:child_process';
import { realpath } from 'node:fs/promises';
import net from 'node:net';

const DEFAULT_HOST = '127.0.0.1';
const DEFAULT_APP_PORT_BASE = 8080;
const DEFAULT_PROXY_PORT_BASE = 7331;
const DEFAULT_SITE_PORT_BASE = 8180;
const DEFAULT_PORT_RANGE = 400;

function usage() {
  return `Usage: npm run dev -- [options] [manja flags]

Options:
  --host <host>          Host for Manja and the Air proxy. Default: 127.0.0.1
  --app-port <port>      Use this exact Manja application port.
  --proxy-port <port>    Use this exact Air proxy port.
  --site-port <port>     Use this exact product site port.
  --print-ports          Print selected ports and exit.
  --help                 Show this help.

Environment:
  MANJA_DEV_HOST              Host override.
  MANJA_DEV_APP_PORT          Exact Manja application port.
  MANJA_DEV_PROXY_PORT        Exact Air proxy port.
  MANJA_DEV_SITE_PORT         Exact product site port.
  MANJA_DEV_APP_PORT_BASE     Start of automatic app port range. Default: 8080
  MANJA_DEV_PROXY_PORT_BASE   Start of automatic proxy port range. Default: 7331
  MANJA_DEV_SITE_PORT_BASE    Start of automatic site port range. Default: 8180
  MANJA_DEV_PORT_RANGE        Automatic range size. Default: 400

Unknown arguments are passed to cmd/manja, for example:
  npm run dev -- -spec internal/adapters/openapi/testdata/petstore.yaml
`;
}

function parsePort(value, name) {
  const port = Number(value);
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error(`${name} must be an integer TCP port between 1 and 65535`);
  }
  return port;
}

function parsePositiveInteger(value, name) {
  const number = Number(value);
  if (!Number.isInteger(number) || number < 1) {
    throw new Error(`${name} must be a positive integer`);
  }
  return number;
}

function parseArgs(argv) {
  const options = {
    host: process.env.MANJA_DEV_HOST || DEFAULT_HOST,
    appPort: process.env.MANJA_DEV_APP_PORT
      ? parsePort(process.env.MANJA_DEV_APP_PORT, 'MANJA_DEV_APP_PORT')
      : null,
    proxyPort: process.env.MANJA_DEV_PROXY_PORT
      ? parsePort(process.env.MANJA_DEV_PROXY_PORT, 'MANJA_DEV_PROXY_PORT')
      : null,
    sitePort: process.env.MANJA_DEV_SITE_PORT
      ? parsePort(process.env.MANJA_DEV_SITE_PORT, 'MANJA_DEV_SITE_PORT')
      : null,
    appPortBase: process.env.MANJA_DEV_APP_PORT_BASE
      ? parsePort(process.env.MANJA_DEV_APP_PORT_BASE, 'MANJA_DEV_APP_PORT_BASE')
      : DEFAULT_APP_PORT_BASE,
    proxyPortBase: process.env.MANJA_DEV_PROXY_PORT_BASE
      ? parsePort(process.env.MANJA_DEV_PROXY_PORT_BASE, 'MANJA_DEV_PROXY_PORT_BASE')
      : DEFAULT_PROXY_PORT_BASE,
    sitePortBase: process.env.MANJA_DEV_SITE_PORT_BASE
      ? parsePort(process.env.MANJA_DEV_SITE_PORT_BASE, 'MANJA_DEV_SITE_PORT_BASE')
      : DEFAULT_SITE_PORT_BASE,
    portRange: process.env.MANJA_DEV_PORT_RANGE
      ? parsePositiveInteger(process.env.MANJA_DEV_PORT_RANGE, 'MANJA_DEV_PORT_RANGE')
      : DEFAULT_PORT_RANGE,
    printPorts: false,
    help: false,
    manjaArgs: [],
  };

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === '--') {
      options.manjaArgs.push(...argv.slice(i + 1));
      break;
    }
    if (arg === '--help' || arg === '-h') {
      options.help = true;
      continue;
    }
    if (arg === '--print-ports') {
      options.printPorts = true;
      continue;
    }
    if (arg === '--host') {
      options.host = argv[++i];
      continue;
    }
    if (arg.startsWith('--host=')) {
      options.host = arg.slice('--host='.length);
      continue;
    }
    if (arg === '--app-port') {
      options.appPort = parsePort(argv[++i], '--app-port');
      continue;
    }
    if (arg.startsWith('--app-port=')) {
      options.appPort = parsePort(arg.slice('--app-port='.length), '--app-port');
      continue;
    }
    if (arg === '--proxy-port') {
      options.proxyPort = parsePort(argv[++i], '--proxy-port');
      continue;
    }
    if (arg.startsWith('--proxy-port=')) {
      options.proxyPort = parsePort(arg.slice('--proxy-port='.length), '--proxy-port');
      continue;
    }
    if (arg === '--site-port') {
      options.sitePort = parsePort(argv[++i], '--site-port');
      continue;
    }
    if (arg.startsWith('--site-port=')) {
      options.sitePort = parsePort(arg.slice('--site-port='.length), '--site-port');
      continue;
    }
    options.manjaArgs.push(arg);
  }

  if (!options.host) {
    throw new Error('--host cannot be empty');
  }
  if (options.appPortBase + options.portRange - 1 > 65535) {
    throw new Error('automatic app port range exceeds 65535');
  }
  if (options.proxyPortBase + options.portRange - 1 > 65535) {
    throw new Error('automatic proxy port range exceeds 65535');
  }
  if (options.sitePortBase + options.portRange - 1 > 65535) {
    throw new Error('automatic site port range exceeds 65535');
  }

  return options;
}

function hashString(value) {
  let hash = 2166136261;
  for (const char of value) {
    hash ^= char.codePointAt(0);
    hash = Math.imul(hash, 16777619);
  }
  return hash >>> 0;
}

function canListen(host, port) {
  return new Promise((resolve) => {
    const server = net.createServer();
    server.unref();
    server.once('error', () => resolve(false));
    server.listen({ host, port }, () => {
      server.close(() => resolve(true));
    });
  });
}

async function assertAvailable(host, port, name) {
  if (!(await canListen(host, port))) {
    throw new Error(`${name} port ${port} is already in use on ${host}`);
  }
}

async function choosePorts(options) {
  if (options.appPort) {
    await assertAvailable(options.host, options.appPort, 'Manja application');
  }
  if (options.proxyPort) {
    await assertAvailable(options.host, options.proxyPort, 'Air proxy');
  }
  if (options.sitePort) {
    await assertAvailable(options.host, options.sitePort, 'Product site');
  }
  if (options.appPort && options.proxyPort && options.sitePort) {
    if (new Set([options.appPort, options.proxyPort, options.sitePort]).size !== 3) {
      throw new Error('app, proxy, and site ports must be different');
    }
    return { appPort: options.appPort, proxyPort: options.proxyPort, sitePort: options.sitePort };
  }

  const cwd = await realpath(process.cwd());
  const offset = hashString(cwd) % options.portRange;
  for (let i = 0; i < options.portRange; i += 1) {
    const candidateOffset = (offset + i) % options.portRange;
    const appPort = options.appPort || options.appPortBase + candidateOffset;
    const proxyPort = options.proxyPort || options.proxyPortBase + candidateOffset;
    const sitePort = options.sitePort || options.sitePortBase + candidateOffset;
    if (new Set([appPort, proxyPort, sitePort]).size !== 3) {
      continue;
    }
    if ((options.appPort || await canListen(options.host, appPort)) &&
        (options.proxyPort || await canListen(options.host, proxyPort)) &&
        (options.sitePort || await canListen(options.host, sitePort))) {
      return { appPort, proxyPort, sitePort };
    }
  }

  throw new Error(
    `no available app/proxy/site port set found in ${options.appPortBase}-${options.appPortBase + options.portRange - 1}, ${options.proxyPortBase}-${options.proxyPortBase + options.portRange - 1}, and ${options.sitePortBase}-${options.sitePortBase + options.portRange - 1}`,
  );
}

function buildAirArgs(options, ports) {
  const appAddr = `${options.host}:${ports.appPort}`;
  const entrypoint = [
    './tmp/manja-dev',
    '-addr',
    appAddr,
    '-data-dir',
    '.manja/data',
    ...options.manjaArgs,
  ].join(',');

  return [
    'run',
    'github.com/air-verse/air@v1.65.3',
    '-c',
    '.air.toml',
    '--build.entrypoint',
    entrypoint,
    '--proxy.app_port',
    String(ports.appPort),
    '--proxy.proxy_port',
    String(ports.proxyPort),
  ];
}

function buildSiteArgs(options, ports) {
  return [
    'run',
    './cmd/server',
    '-addr',
    `${options.host}:${ports.sitePort}`,
  ];
}

function spawnAir(options, ports) {
  const proxyURL = `http://${options.host}:${ports.proxyPort}`;
  const appURL = `http://${options.host}:${ports.appPort}`;
  const siteURL = `http://${options.host}:${ports.sitePort}`;
  const airArgs = buildAirArgs(options, ports);
  const siteArgs = buildSiteArgs(options, ports);

  console.log(`Manja app: ${appURL}`);
  console.log(`Air reload proxy: ${proxyURL}`);
  console.log(`Manja site: ${siteURL}`);
  console.log(`Open ${siteURL}`);

  const air = spawn('go', airArgs, {
    stdio: 'inherit',
    env: {
      ...process.env,
      MANJA_DEV_APP_URL: appURL,
      MANJA_DEV_PROXY_URL: proxyURL,
      MANJA_DEV_SITE_URL: siteURL,
    },
  });
  const site = spawn('go', siteArgs, { stdio: 'inherit', cwd: 'site' });

  let exiting = false;
  function shutdown(signal) {
    if (exiting) {
      return;
    }
    exiting = true;
    air.kill(signal);
    site.kill(signal);
  }

  for (const signal of ['SIGINT', 'SIGTERM']) {
    process.on(signal, () => {
      shutdown(signal);
    });
  }

  function exitFromChild(code, signal) {
    const signalExitCodes = { SIGINT: 130, SIGTERM: 143 };
    shutdown(signal || 'SIGTERM');
    process.exit(code ?? signalExitCodes[signal] ?? 1);
  }

  air.on('exit', exitFromChild);
  site.on('exit', exitFromChild);
}

try {
  const options = parseArgs(process.argv.slice(2));
  if (options.help) {
    console.log(usage());
    process.exit(0);
  }

  const ports = await choosePorts(options);
  if (options.printPorts) {
    console.log(JSON.stringify({
      appPort: ports.appPort,
      proxyPort: ports.proxyPort,
      sitePort: ports.sitePort,
      appURL: `http://${options.host}:${ports.appPort}`,
      proxyURL: `http://${options.host}:${ports.proxyPort}`,
      siteURL: `http://${options.host}:${ports.sitePort}`,
      airArgs: buildAirArgs(options, ports),
      siteArgs: buildSiteArgs(options, ports),
    }, null, 2));
    process.exit(0);
  }

  spawnAir(options, ports);
} catch (error) {
  console.error(error instanceof Error ? error.message : error);
  process.exit(1);
}
