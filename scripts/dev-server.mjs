#!/usr/bin/env node
import { spawn } from 'node:child_process';
import { realpath } from 'node:fs/promises';
import net from 'node:net';

const DEFAULT_HOST = '127.0.0.1';
const DEFAULT_APP_PORT_BASE = 8080;
const DEFAULT_PROXY_PORT_BASE = 7331;
const DEFAULT_PORT_RANGE = 400;

function usage() {
  return `Usage: npm run dev -- [options] [manja flags]

Options:
  --host <host>          Host for Manja and the Air proxy. Default: 127.0.0.1
  --app-port <port>      Use this exact Manja application port.
  --proxy-port <port>    Use this exact Air proxy port.
  --print-ports          Print selected ports and exit.
  --help                 Show this help.

Environment:
  MANJA_DEV_HOST              Host override.
  MANJA_DEV_APP_PORT          Exact Manja application port.
  MANJA_DEV_PROXY_PORT        Exact Air proxy port.
  MANJA_DEV_APP_PORT_BASE     Start of automatic app port range. Default: 8080
  MANJA_DEV_PROXY_PORT_BASE   Start of automatic proxy port range. Default: 7331
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
    appPortBase: process.env.MANJA_DEV_APP_PORT_BASE
      ? parsePort(process.env.MANJA_DEV_APP_PORT_BASE, 'MANJA_DEV_APP_PORT_BASE')
      : DEFAULT_APP_PORT_BASE,
    proxyPortBase: process.env.MANJA_DEV_PROXY_PORT_BASE
      ? parsePort(process.env.MANJA_DEV_PROXY_PORT_BASE, 'MANJA_DEV_PROXY_PORT_BASE')
      : DEFAULT_PROXY_PORT_BASE,
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
  if (options.appPort && options.proxyPort) {
    if (options.appPort === options.proxyPort) {
      throw new Error('app and proxy ports must be different');
    }
    return { appPort: options.appPort, proxyPort: options.proxyPort };
  }

  const cwd = await realpath(process.cwd());
  const offset = hashString(cwd) % options.portRange;
  for (let i = 0; i < options.portRange; i += 1) {
    const candidateOffset = (offset + i) % options.portRange;
    const appPort = options.appPort || options.appPortBase + candidateOffset;
    const proxyPort = options.proxyPort || options.proxyPortBase + candidateOffset;
    if (appPort === proxyPort) {
      continue;
    }
    if ((options.appPort || await canListen(options.host, appPort)) &&
        (options.proxyPort || await canListen(options.host, proxyPort))) {
      return { appPort, proxyPort };
    }
  }

  throw new Error(
    `no available app/proxy port pair found in ${options.appPortBase}-${options.appPortBase + options.portRange - 1} and ${options.proxyPortBase}-${options.proxyPortBase + options.portRange - 1}`,
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

function spawnAir(options, ports) {
  const proxyURL = `http://${options.host}:${ports.proxyPort}`;
  const appURL = `http://${options.host}:${ports.appPort}`;
  const airArgs = buildAirArgs(options, ports);

  console.log(`Manja app: ${appURL}`);
  console.log(`Air reload proxy: ${proxyURL}`);
  console.log(`Open ${proxyURL}`);

  const child = spawn('go', airArgs, {
    stdio: 'inherit',
    env: {
      ...process.env,
      MANJA_DEV_APP_URL: appURL,
      MANJA_DEV_PROXY_URL: proxyURL,
    },
  });

  for (const signal of ['SIGINT', 'SIGTERM']) {
    process.on(signal, () => {
      child.kill(signal);
    });
  }

  child.on('exit', (code, signal) => {
    const signalExitCodes = { SIGINT: 130, SIGTERM: 143 };
    process.exit(code ?? signalExitCodes[signal] ?? 1);
  });
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
      appURL: `http://${options.host}:${ports.appPort}`,
      proxyURL: `http://${options.host}:${ports.proxyPort}`,
      airArgs: buildAirArgs(options, ports),
    }, null, 2));
    process.exit(0);
  }

  spawnAir(options, ports);
} catch (error) {
  console.error(error instanceof Error ? error.message : error);
  process.exit(1);
}
