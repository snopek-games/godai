#!/usr/bin/env node
// Generates the publishable npm packages into npm/dist/ from the Go binaries
// that CI builds into dist/mcp/.
//
// Usage: node npm/prepare-packages.mjs <version>
//
// Produces:
//   npm/dist/godai-mcp/            - the main launcher package
//   npm/dist/platforms/<name>/     - one package per platform binary
//
// The platform packages listed in npm/package.json's optionalDependencies are
// the source of truth for which platforms exist; this script maps each one to
// the corresponding CI build variant via VARIANTS below.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const NPM_DIR = path.dirname(fileURLToPath(import.meta.url));
const ROOT_DIR = path.dirname(NPM_DIR);
const BUILD_DIR = path.join(ROOT_DIR, 'dist', 'mcp');
const OUT_DIR = path.join(NPM_DIR, 'dist');

const APP_NAME = 'godai-mcp';

// Maps npm package name -> the VARIANT used by the mcp-build CI job.
const VARIANTS = {
  '@snopek-games/godai-mcp-linux-x64': { variant: 'linux-x86_64', ext: '' },
  '@snopek-games/godai-mcp-linux-arm64': { variant: 'linux-arm64', ext: '' },
  '@snopek-games/godai-mcp-darwin-arm64': { variant: 'macos-arm64', ext: '' },
  '@snopek-games/godai-mcp-win32-x64': { variant: 'windows-x86_64', ext: '.exe' },
  '@snopek-games/godai-mcp-win32-arm64': { variant: 'windows-arm64', ext: '.exe' },
};

function fail(message) {
  console.error(`prepare-packages: ${message}`);
  process.exit(1);
}

const version = process.argv[2];
if (!version || !/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version)) {
  fail(`usage: node prepare-packages.mjs <version> (got: ${JSON.stringify(version)})`);
}

const basePackage = JSON.parse(fs.readFileSync(path.join(NPM_DIR, 'package.json'), 'utf8'));
const licenseSrc = path.join(ROOT_DIR, 'LICENSE.txt');

fs.rmSync(OUT_DIR, { recursive: true, force: true });

// Generate the platform-specific packages.
const optionalDependencies = {};
for (const packageName of Object.keys(basePackage.optionalDependencies)) {
  const info = VARIANTS[packageName];
  if (!info) {
    fail(`no VARIANTS entry for "${packageName}" (declared in npm/package.json optionalDependencies)`);
  }

  // Package names follow the pattern <scope>/godai-mcp-<os>-<cpu>.
  const [os, cpu] = packageName.slice(basePackage.name.length + 1).split('-');

  const variantDir = `${APP_NAME}-${info.variant}`;
  const binarySrc = path.join(BUILD_DIR, variantDir, `${variantDir}${info.ext}`);
  if (!fs.existsSync(binarySrc)) {
    fail(`missing binary ${binarySrc} - did the mcp-build job run for all platforms?`);
  }

  // Directory name without the scope, to avoid a nested "@snopek-games" dir.
  const packageDir = path.join(OUT_DIR, 'platforms', packageName.split('/').pop());
  fs.mkdirSync(packageDir, { recursive: true });

  fs.writeFileSync(path.join(packageDir, 'package.json'), JSON.stringify({
    name: packageName,
    version: version,
    description: `${os}-${cpu} binary for ${APP_NAME}`,
    license: basePackage.license,
    homepage: basePackage.homepage,
    repository: basePackage.repository,
    os: [os],
    cpu: [cpu],
    preferUnplugged: true,
  }, null, 2) + '\n');

  const binaryDest = path.join(packageDir, `${APP_NAME}${info.ext}`);
  fs.copyFileSync(binarySrc, binaryDest);
  fs.chmodSync(binaryDest, 0o755);
  fs.copyFileSync(licenseSrc, path.join(packageDir, 'LICENSE.txt'));

  optionalDependencies[packageName] = version;
  console.log(`generated ${packageName}@${version}`);
}

// Generate the main package, with the version stamped into it and its
// optionalDependencies pinned to the exact same version.
const mainDir = path.join(OUT_DIR, APP_NAME);
fs.mkdirSync(mainDir, { recursive: true });

basePackage.version = version;
basePackage.optionalDependencies = optionalDependencies;
fs.writeFileSync(path.join(mainDir, 'package.json'), JSON.stringify(basePackage, null, 2) + '\n');

const launcherDest = path.join(mainDir, `${APP_NAME}.js`);
fs.copyFileSync(path.join(NPM_DIR, `${APP_NAME}.js`), launcherDest);
fs.chmodSync(launcherDest, 0o755);
fs.copyFileSync(path.join(NPM_DIR, 'README.md'), path.join(mainDir, 'README.md'));
fs.copyFileSync(licenseSrc, path.join(mainDir, 'LICENSE.txt'));

console.log(`generated ${APP_NAME}@${version}`);
