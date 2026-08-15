#!/usr/bin/env node
'use strict';

// Launcher for the godai binary. The actual binary ships in one of the
// platform-specific packages listed in optionalDependencies; npm only
// installs the one whose "os"/"cpu" fields match the current system.

const { spawnSync } = require('node:child_process');
const path = require('node:path');

function fail(message) {
  console.error(`godai: ${message}`);
  process.exit(1);
}

const packageName = `@snopek-games/godai-${process.platform}-${process.arch}`;
const supported = Object.keys(require('./package.json').optionalDependencies || {});

if (!supported.includes(packageName)) {
  fail(
    `unsupported platform "${process.platform}-${process.arch}".\n` +
    `Supported platforms: ${supported.map((name) => name.replace(/^.*\/godai-/, '')).join(', ')}\n` +
    'Prebuilt binaries for other platforms are available at https://gitlab.com/snopek-games/godai/-/releases'
  );
}

const binaryName = process.platform === 'win32' ? 'godai.exe' : 'godai';

let binaryPath;
try {
  binaryPath = path.join(path.dirname(require.resolve(`${packageName}/package.json`)), binaryName);
} catch {
  fail(
    `the "${packageName}" package with the binary for your platform is not installed.\n` +
    'It should have been installed automatically as an optional dependency. If you installed\n' +
    'with --no-optional or --omit=optional, reinstall without that flag.'
  );
}

const result = spawnSync(binaryPath, process.argv.slice(2), { stdio: 'inherit' });
if (result.error) {
  fail(`failed to run ${binaryPath}: ${result.error.message}`);
}
process.exit(result.status === null ? 1 : result.status);
