#!/usr/bin/env node
// Stamps the given version into packaging/mcpb/manifest.json, which is
// committed with a 0.0.0-dev placeholder (the version in addons/godai/plugin.cfg
// is the single source of truth). CI runs this before `mcpb pack`.
//
// This script is kept out of the packed bundle by packaging/mcpb/.mcpbignore.
//
// Usage: node packaging/mcpb/set-bundle-version.mjs <version>

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BUNDLE_DIR = path.dirname(fileURLToPath(import.meta.url));
const MANIFEST_PATH = path.join(BUNDLE_DIR, 'manifest.json');

function fail(message) {
  console.error(`set-bundle-version: ${message}`);
  process.exit(1);
}

const version = process.argv[2];
if (!version || !/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version)) {
  fail(`usage: node packaging/mcpb/set-bundle-version.mjs <version> (got: ${JSON.stringify(version)})`);
}

const manifest = JSON.parse(fs.readFileSync(MANIFEST_PATH, 'utf8'));
manifest.version = version;
fs.writeFileSync(MANIFEST_PATH, JSON.stringify(manifest, null, 2) + '\n');

console.log(`set-bundle-version: stamped version ${version} into packaging/mcpb/manifest.json`);
