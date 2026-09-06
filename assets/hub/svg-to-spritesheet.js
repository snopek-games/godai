#!/usr/bin/env node
// Rasterizes a SMIL-animated SVG into a transparent PNG spritesheet by rendering every
// frame in one headless Chrome page. The sheet width is a power of 2; the height is
// only as tall as the frames need.
//
// Usage:  node svg-to-spritesheet.js <in.svg> <out.png> [options]
//   --fps N        frames per second (default 30)
//   --size N       frame size in pixels, square (default 64)
//   --duration S   loop length in seconds (default: longest dur="" in the SVG)
//   --frames N     total frames (default: round(fps * duration))
//   --chrome PATH  Chrome/Chromium binary (default: $CHROME, else searched on PATH)
//
// Frames are laid out row-major, left to right, top to bottom.

const fs = require('fs');
const os = require('os');
const path = require('path');
const { execFileSync } = require('child_process');

function parseArgs(argv) {
  const opts = { fps: 30, size: 64, duration: null, frames: null, chrome: process.env.CHROME || null };
  const positional = [];
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (!arg.startsWith('--')) { positional.push(arg); continue; }
    const key = arg.slice(2);
    if (!(key in opts)) fail(`unknown option ${arg}`);
    const value = argv[++i];
    if (value === undefined) fail(`${arg} needs a value`);
    opts[key] = key === 'chrome' ? value : Number(value);
    if (key !== 'chrome' && !(opts[key] > 0)) fail(`${arg} must be a positive number`);
  }
  if (positional.length !== 2) fail('expected <in.svg> <out.png>');
  return { input: positional[0], output: positional[1], ...opts };
}

function fail(message) {
  console.error(`error: ${message}`);
  console.error('usage: svg-to-spritesheet.js <in.svg> <out.png> [--fps N] [--size N] [--duration S] [--frames N] [--chrome PATH]');
  process.exit(1);
}

function longestDuration(svg) {
  let longest = 0;
  for (const match of svg.matchAll(/\bdur="([\d.]+)(m?s)"/g)) {
    const seconds = Number(match[1]) / (match[2] === 'ms' ? 1000 : 1);
    longest = Math.max(longest, seconds);
  }
  if (!longest) fail('no dur="..." found in SVG; pass --duration');
  return longest;
}

function nextPowerOfTwo(n) {
  return 2 ** Math.ceil(Math.log2(n));
}

function findChrome(explicit) {
  const candidates = explicit ? [explicit] : ['google-chrome', 'google-chrome-stable', 'chromium', 'chromium-browser', 'chrome'];
  for (const candidate of candidates) {
    try {
      execFileSync(candidate, ['--version'], { stdio: 'ignore' });
      return candidate;
    } catch {}
  }
  fail('Chrome/Chromium not found; set $CHROME or pass --chrome');
}

function buildPage(svg, { frames, cols, size, width, height, duration }) {
  return `<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;padding:0;background:transparent;overflow:hidden}
body{width:${width}px;height:${height}px;position:relative}
svg{position:absolute;display:block}
</style></head><body>
<template id="frame">${svg}</template>
<script>
const template = document.getElementById('frame').content.firstElementChild;
for (let i = 0; i < ${frames}; i++) {
  const el = template.cloneNode(true);
  el.setAttribute('width', ${size});
  el.setAttribute('height', ${size});
  el.style.left = (i % ${cols}) * ${size} + 'px';
  el.style.top = Math.floor(i / ${cols}) * ${size} + 'px';
  document.body.appendChild(el);
  el.pauseAnimations();
  el.setCurrentTime(i * ${duration} / ${frames});
}
</script></body></html>`;
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  const svg = fs.readFileSync(args.input, 'utf8');
  const duration = args.duration ?? longestDuration(svg);
  const frames = args.frames ?? Math.round(args.fps * duration);
  const width = nextPowerOfTwo(Math.ceil(Math.sqrt(frames)) * args.size);
  const cols = width / args.size;
  const rows = Math.ceil(frames / cols);
  const height = rows * args.size;
  const layout = { frames, cols, rows, size: args.size, width, height, duration };

  const workDir = fs.mkdtempSync(path.join(os.tmpdir(), 'svg-spritesheet-'));
  const pagePath = path.join(workDir, 'sheet.html');
  const outputPath = path.resolve(args.output);
  fs.writeFileSync(pagePath, buildPage(svg, layout));
  try {
    execFileSync(findChrome(args.chrome), [
      '--headless',
      '--disable-gpu',
      '--hide-scrollbars',
      '--force-device-scale-factor=1',
      '--default-background-color=00000000',
      `--window-size=${width},${height}`,
      `--screenshot=${outputPath}`,
      `file://${pagePath}`,
    ], { stdio: ['ignore', 'ignore', 'inherit'] });
  } finally {
    fs.rmSync(workDir, { recursive: true, force: true });
  }

  console.log(`${outputPath}: ${width}x${height}, ${frames} frames of ${args.size}x${args.size} in ${cols} columns x ${rows} rows (${cols * rows - frames} cells unused)`);
  console.log(`loop: ${duration}s at ${(frames / duration).toFixed(3)} fps`);
}

main();
