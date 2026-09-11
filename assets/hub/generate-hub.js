#!/usr/bin/env node
// godai "active hub" generator
// -----------------------------------------------
// Emits a family of self-contained SVGs (SMIL-baked animation, no JavaScript
// in the output).
//
// Motion (all seamless over SECONDS_PER_LOOP):
//   - idle: gentle 2-harmonic sine drift
//   - activated: fractal noise sampled along a closed circle in noise space
//   - a per-node sharpened-sine activation wave crossfades the two, staggered
//     so nodes take turns "lighting up" (activated nodes also swell a bit)
//   - the hub breathes on its own independent sine
//
// Variants (see VARIANTS at the bottom)
//   themes:  light / dark            (colors in THEMES)
//   bg:      tile / transparent      (ring lines are foreground at
//                                     `ringOpacity`, so they adapt to whatever
//                                     sits behind them — tile or the page)
//   size:    normal / icon           (icon = chunkier shapes for tiny sizes)
//   motion:  animated / static       (static pose set by STATIC_POSE_TIME)
//
// Usage:  node hub-animated-noise-gen.js [output-dir]
//
// ============================ KNOBS =====================================

// ---- static pose: a time in seconds (0..SECONDS_PER_LOOP), or 'rest' for
// the pose with every node at its origin, base radius, hub at mid-breath.
//const STATIC_POSE_TIME = 'rest';
const STATIC_POSE_TIME = 2.825;
//const STATIC_POSE_TIME = 6.0;

// ---- colors.
// `tile` is a hex, or `{ top: '#hex', bottom: '#hex' }` for a vertical gradient.
// ring lines default to `foreground` at `ringOpacity`, so they
// blend with whatever is behind them (the tile, or the page in -nobg
// variants). Set `ringColor` to an explicit hex to make them a solid,
// background-independent color instead.
const THEMES = {
  light: {
    tile: {
      top: '#2d4252',
      //top: '#273039',
      bottom: '#1c1f22',
    },
    foreground: '#ffffff',
    // ring-line opacity (foreground over whatever's behind)
    ringOpacity: 0.45,
    // e.g. '#9ac0dc' for a solid, opaque override
    ringColor: null,
    // preview.html only: what -nobg variants are shown against
    testBackground: '#000000',
  },
  dark: {
    tile:       '#478cbf',
    foreground: '#123153',
    ringOpacity: 0.45,
    ringColor: null,
    testBackground: '#ffffff',
  },
};

// ---- geometry per size.
// A size may also set `ringOpacity` to override the theme's.
const SIZES = {
  normal: {
    // distance from the center to the five outer nodes (pentagon radius)
    ringRadius: 86,
    // base radius of the outer node dots (swells by ACTIVATION_RADIUS_BOOST)
    nodeRadius: 17.5,
    // center hub radius at the bottom of a breath
    hubMinRadius: 16.5,
    // center hub radius at the top of a breath
    hubMaxRadius: 21.0,
    // stroke width of the hub-to-node spokes
    spokeWidth: 8.5,
    // stroke width of the node-to-node ring lines
    ringWidth: 7.5
  },
  // Bigger shapes so it's more readable at a smaller size.
  icon: {
    ringRadius: 82,
    nodeRadius: 20,
    hubMinRadius: 19,
    hubMaxRadius: 24,
    spokeWidth: 13,
    ringWidth: 13,
    ringOpacity: 0.8
  },
};

// ---- animation ----
const SECONDS_PER_LOOP = 6;
const SAMPLES_PER_SECOND = 40; // baked into the SVG (raise with OCTAVES/LACUNARITY)
const SMOOTH_AMPLITUDE = 5;    // peak idle drift, px
const JITTER_AMPLITUDE = 9;    // peak activated excursion, px
const OCTAVES = 4;             // noise layers in the jitter
const PERSISTENCE = 0.55;      // energy in fast noise layers (0.4 calm - 0.7 fizzy)
const LACUNARITY = 2.1;        // frequency ratio between noise layers
const LOOP_RADIUS = 1.5;       // noise-space circle radius; bigger = busier jitter
const ACTIVATIONS_PER_LOOP = [2, 3, 2, 3, 2]; // bursts/loop per node — keep INTEGERS
const ACTIVATION_SHARPNESS = 5;    // higher = shorter, punchier bursts
const ACTIVATION_RADIUS_BOOST = 0.18; // node radius swell at full activation (0 = off)
const HUB_BREATHS_PER_LOOP = 2;    // keep INTEGER
const SEED = 20260817;             // same seed = same performance in every variant

// ---- canvas ----
const CANVAS_SIZE = 256;
const CENTER_X = 128, CENTER_Y = 124;
const TILE = { cornerRadius: 56, inset: 4 };

// ============================ VARIANTS ==================================
// {file, theme, bg, size, static, margin, preview} — comment out lines you
// don't want. margin = transparent px added around the canvas on every side.
// preview: false leaves the variant out of preview.html.
const S = STATIC_POSE_TIME;
const VARIANTS = [
  { file: 'godai-hub.svg',                       theme: 'light', bg: true,  size: 'normal' },
  { file: 'godai-hub-margin.svg',                theme: 'light', bg: true,  size: 'normal', margin: 16, preview: false },
  { file: 'godai-hub-icon.svg',                  theme: 'light', bg: true,  size: 'icon' },
  { file: 'godai-hub-static.svg',                theme: 'light', bg: true,  size: 'normal', static: S },
  { file: 'godai-hub-static-icon.svg',           theme: 'light', bg: true,  size: 'icon',   static: S },
  { file: 'godai-hub-nobg.svg',                  theme: 'light', bg: false, size: 'normal' },
  { file: 'godai-hub-nobg-icon.svg',             theme: 'light', bg: false, size: 'icon' },
  { file: 'godai-hub-nobg-static.svg',           theme: 'light', bg: false, size: 'normal', static: S },
  { file: 'godai-hub-nobg-static-icon.svg',      theme: 'light', bg: false, size: 'icon',   static: S },
  { file: 'godai-hub-dark-nobg.svg',             theme: 'dark',  bg: false, size: 'normal' },
  { file: 'godai-hub-dark-nobg-icon.svg',        theme: 'dark',  bg: false, size: 'icon' },
  { file: 'godai-hub-dark-nobg-static.svg',      theme: 'dark',  bg: false, size: 'normal', static: S },
  { file: 'godai-hub-dark-nobg-static-icon.svg', theme: 'dark',  bg: false, size: 'icon',   static: S },
];
// ========================================================================

const fs = require('fs');
const path = require('path');
const SAMPLE_COUNT = Math.round(SECONDS_PER_LOOP * SAMPLES_PER_SECOND);

// ---- color helpers ----
function hex2rgb(h) {
  const n = parseInt(h.slice(1), 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}
function rgb2hex([r, g, b]) {
  return '#' + [r, g, b].map(v => Math.round(v).toString(16).padStart(2, '0')).join('');
}
// alpha-composite a over b:  a*t + b*(1-t)
function mix(a, b, t) {
  const A = hex2rgb(a), B = hex2rgb(b);
  return rgb2hex(A.map((v, i) => v * t + B[i] * (1 - t)));
}

// ---- tiny deterministic PRNG (mulberry32) ----
function rng(seed) {
  return function () {
    seed |= 0; seed = (seed + 0x6D2B79F5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

// ---- motion tracks; built once, shared by every variant ----
// (offsets and weights are geometry-independent, so normal and icon sizes
// perform the exact same dance)
function buildTracks() {
  const rand = rng(SEED);

  // 2D gradient noise + fBm
  const perm = new Uint8Array(512);
  {
    const p = [...Array(256).keys()];
    for (let i = 255; i > 0; i--) {
      const j = Math.floor(rand() * (i + 1));
      [p[i], p[j]] = [p[j], p[i]];
    }
    for (let i = 0; i < 512; i++) perm[i] = p[i & 255];
  }
  const grad = (h, x, y) => {
    switch (h & 7) {
      case 0: return  x + y;  case 1: return  x - y;
      case 2: return -x + y;  case 3: return -x - y;
      case 4: return  x;      case 5: return -x;
      case 6: return  y;      default: return -y;
    }
  };
  const fade = t => t * t * t * (t * (t * 6 - 15) + 10);
  const lerp = (a, b, t) => a + t * (b - a);
  function noise2(x, y) {
    const X = Math.floor(x) & 255, Y = Math.floor(y) & 255;
    x -= Math.floor(x); y -= Math.floor(y);
    const u = fade(x), v = fade(y);
    const aa = perm[perm[X] + Y],     ab = perm[perm[X] + Y + 1];
    const ba = perm[perm[X + 1] + Y], bb = perm[perm[X + 1] + Y + 1];
    return lerp(
      lerp(grad(aa, x, y),     grad(ba, x - 1, y),     u),
      lerp(grad(ab, x, y - 1), grad(bb, x - 1, y - 1), u), v);
  }
  function fbm(x, y) {
    let sum = 0, amp = 1, freq = 1;
    for (let o = 0; o < OCTAVES; o++) {
      sum += amp * noise2(x * freq, y * freq);
      amp *= PERSISTENCE;
      freq *= LACUNARITY;
    }
    return sum;
  }

  // i=SAMPLE_COUNT wraps to i=0: closed loop
  const timeAt = i => ((i % SAMPLE_COUNT) * SECONDS_PER_LOOP) / SAMPLE_COUNT;
  const samples = fn => {
    const out = [];
    for (let i = 0; i <= SAMPLE_COUNT; i++) out.push(fn(timeAt(i)));
    return out;
  };
  const normalize = (raw, amplitude) => {
    const mean = raw.reduce((a, b) => a + b, 0) / raw.length;
    let peak = 1e-9;
    for (const v of raw) peak = Math.max(peak, Math.abs(v - mean));
    return raw.map(v => ((v - mean) / peak) * amplitude);
  };
  const jitterTrack = amplitude => {
    const originX = rand() * 256, originY = rand() * 256;
    return normalize(samples(t => {
      const angle = (2 * Math.PI * t) / SECONDS_PER_LOOP;
      return fbm(originX + LOOP_RADIUS * Math.cos(angle), originY + LOOP_RADIUS * Math.sin(angle));
    }), amplitude);
  };
  const smoothTrack = amplitude => {
    const harmonics = [1, 2].map(k => ({ k, amp: (rand() * 0.7 + 0.3) / k, phase: rand() * Math.PI * 2 }));
    return normalize(samples(t =>
      harmonics.reduce((sum, h) => sum + h.amp * Math.sin((2 * Math.PI * h.k * t) / SECONDS_PER_LOOP + h.phase), 0)), amplitude);
  };
  const activationTrack = cycles => {
    const phase = rand() * Math.PI * 2;
    return samples(t =>
      Math.pow((Math.sin((2 * Math.PI * cycles * t) / SECONDS_PER_LOOP + phase) + 1) / 2, ACTIVATION_SHARPNESS));
  };

  // per outer node: offset tracks + activation weight
  const nodes = [];
  for (let i = 0; i < 5; i++) {
    const activation = activationTrack(ACTIVATIONS_PER_LOOP[i % ACTIVATIONS_PER_LOOP.length]);
    const smoothX = smoothTrack(SMOOTH_AMPLITUDE), smoothY = smoothTrack(SMOOTH_AMPLITUDE);
    const jitterX = jitterTrack(JITTER_AMPLITUDE), jitterY = jitterTrack(JITTER_AMPLITUDE);
    nodes.push({
      offsetX: activation.map((a, s) => (1 - a) * smoothX[s] + a * jitterX[s]),
      offsetY: activation.map((a, s) => (1 - a) * smoothY[s] + a * jitterY[s]),
      activation,
    });
  }

  // hub breath, 0..1 (starts at the bottom of a breath)
  const breath = samples(t =>
    (Math.sin((2 * Math.PI * HUB_BREATHS_PER_LOOP * t) / SECONDS_PER_LOOP - Math.PI / 2) + 1) / 2);

  return { nodes, breath };
}

// ---- build one SVG variant ----
function buildSvg(tracks, { theme, bg, size, static: staticAt, margin = 0 }) {
  const colors = THEMES[theme];
  const geometry = SIZES[size];
  // solid override, or foreground + opacity (adapts to the background)
  const ringOpacity = geometry.ringOpacity ?? colors.ringOpacity;
  const ringStroke = colors.ringColor
    ? `stroke="${colors.ringColor}"`
    : `stroke="${colors.foreground}" opacity="${ringOpacity}"`;

  // base node positions (pentagon, point-down)
  const basePositions = [];
  for (let i = 0; i < 5; i++) {
    const angle = ((90 + i * 72) * Math.PI) / 180;
    basePositions.push([
      CENTER_X + geometry.ringRadius * Math.cos(angle),
      CENTER_Y + geometry.ringRadius * Math.sin(angle),
    ]);
  }
  const hubRestRadius = (geometry.hubMinRadius + geometry.hubMaxRadius) / 2;

  // resolve tracks into per-node x/y/radius value arrays (or a single
  // static sample)
  let nodeXs, nodeYs, nodeRadii, hubRadii;
  if (staticAt !== undefined) {
    if (staticAt === 'rest') {
      nodeXs = basePositions.map(b => [b[0]]);
      nodeYs = basePositions.map(b => [b[1]]);
      nodeRadii = basePositions.map(() => [geometry.nodeRadius]);
      hubRadii = [hubRestRadius];
    } else {
      const s = Math.round((staticAt % SECONDS_PER_LOOP) / SECONDS_PER_LOOP * SAMPLE_COUNT);
      nodeXs = tracks.nodes.map((n, i) => [+(basePositions[i][0] + n.offsetX[s]).toFixed(1)]);
      nodeYs = tracks.nodes.map((n, i) => [+(basePositions[i][1] + n.offsetY[s]).toFixed(1)]);
      nodeRadii = tracks.nodes.map(n => [+(geometry.nodeRadius * (1 + ACTIVATION_RADIUS_BOOST * n.activation[s])).toFixed(1)]);
      hubRadii = [+(geometry.hubMinRadius + (geometry.hubMaxRadius - geometry.hubMinRadius) * tracks.breath[s]).toFixed(1)];
    }
  } else {
    nodeXs = tracks.nodes.map((n, i) => n.offsetX.map(v => +(basePositions[i][0] + v).toFixed(1)));
    nodeYs = tracks.nodes.map((n, i) => n.offsetY.map(v => +(basePositions[i][1] + v).toFixed(1)));
    nodeRadii = tracks.nodes.map(n => n.activation.map(a => +(geometry.nodeRadius * (1 + ACTIVATION_RADIUS_BOOST * a)).toFixed(1)));
    hubRadii = tracks.breath.map(b => +(geometry.hubMinRadius + (geometry.hubMaxRadius - geometry.hubMinRadius) * b).toFixed(1));
  }

  const anim = (attr, values) => values.length > 1
    ? `<animate attributeName="${attr}" dur="${SECONDS_PER_LOOP}s" repeatCount="indefinite" values="${values.join(';')}"/>`
    : '';

  let body = '';
  if (bg) {
    let tileFill = colors.tile;
    if (typeof colors.tile === 'object') {
      const gradientId = `tile-gradient-${theme}`;
      body += `<defs><linearGradient id="${gradientId}" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="${colors.tile.top}"/><stop offset="1" stop-color="${colors.tile.bottom}"/></linearGradient></defs>\n`;
      tileFill = `url(#${gradientId})`;
    }
    body += `<rect x="${TILE.inset}" y="${TILE.inset}" width="${CANVAS_SIZE - 2 * TILE.inset}" height="${CANVAS_SIZE - 2 * TILE.inset}" rx="${TILE.cornerRadius}" fill="${tileFill}"/>\n`;
  }
  // ring lines (translucent by default, so they mix with the background)
  for (let i = 0; i < 5; i++) {
    const j = (i + 1) % 5;
    body += `<line ${ringStroke} stroke-width="${geometry.ringWidth}" x1="${nodeXs[i][0]}" y1="${nodeYs[i][0]}" x2="${nodeXs[j][0]}" y2="${nodeYs[j][0]}">
  ${anim('x1', nodeXs[i])}${anim('y1', nodeYs[i])}${anim('x2', nodeXs[j])}${anim('y2', nodeYs[j])}
</line>\n`;
  }
  // spokes anchored to the fixed hub
  for (let i = 0; i < 5; i++) {
    body += `<line stroke="${colors.foreground}" stroke-width="${geometry.spokeWidth}" x1="${CENTER_X}" y1="${CENTER_Y}" x2="${nodeXs[i][0]}" y2="${nodeYs[i][0]}">
  ${anim('x2', nodeXs[i])}${anim('y2', nodeYs[i])}
</line>\n`;
  }
  // node dots, then the breathing hub
  for (let i = 0; i < 5; i++) {
    body += `<circle fill="${colors.foreground}" cx="${nodeXs[i][0]}" cy="${nodeYs[i][0]}" r="${nodeRadii[i][0]}">
  ${anim('cx', nodeXs[i])}${anim('cy', nodeYs[i])}${anim('r', nodeRadii[i])}
</circle>\n`;
  }
  body += `<circle fill="${colors.foreground}" cx="${CENTER_X}" cy="${CENTER_Y}" r="${hubRadii[0]}">
  ${anim('r', hubRadii)}
</circle>\n`;

  const outerSize = CANVAS_SIZE + 2 * margin;
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${-margin} ${-margin} ${outerSize} ${outerSize}" width="${outerSize}" height="${outerSize}">
<title>godai — active hub</title>
${body}</svg>`;
}

// ---- preview page: every variant in one grid ----
// rows = theme + background, columns = size + motion. Transparent variants
// sit on their theme's `testBackground`; tile variants sit on a checkerboard
// so the rounded corners stay visible. Animated variants are inlined (not
// <img>) so the scrub bar can drive their SMIL timelines to pick a
// STATIC_POSE_TIME.
function buildPreviewHtml(svgByFile) {
  const motion = v => (v.static !== undefined ? 'static' : 'animated');
  const rowKeys = [], colKeys = [], cells = new Map();
  for (const v of VARIANTS) {
    if (v.preview === false) continue;
    const rowKey = `${v.theme} · ${v.bg ? 'tile' : 'transparent'}`;
    const colKey = `${v.size} · ${motion(v)}`;
    if (!rowKeys.includes(rowKey)) rowKeys.push(rowKey);
    if (!colKeys.includes(colKey)) colKeys.push(colKey);
    const key = `${rowKey}|${colKey}`;
    if (!cells.has(key)) cells.set(key, []);
    cells.get(key).push(v);
  }

  const swatch = v => {
    const cls = ` class="swatch${v.bg ? ' checker' : ''}"`;
    const style = v.bg ? '' : ` style="background:${THEMES[v.theme].testBackground}"`;
    const sizes = v.size === 'icon' ? [128, 48, 32, 24, 16] : [128];
    const outerSize = CANVAS_SIZE + 2 * (v.margin || 0);
    const imgs = sizes.map(s => v.static !== undefined
      ? `<img src="${v.file}" width="${s}" height="${s}" alt="${v.file}">`
      : svgByFile.get(v.file).replace(`width="${outerSize}" height="${outerSize}"`, `class="anim" width="${s}" height="${s}"`)
    ).join('');
    return `<div${cls}${style}>${imgs}</div><div class="name">${v.file}</div>`;
  };
  const cell = vs => (vs ? `<td>${vs.map(swatch).join('')}</td>` : '<td></td>');

  const header = colKeys.map(c => `<th>${c}</th>`).join('');
  const rows = rowKeys.map(rowKey =>
    `<tr><th>${rowKey}</th>${colKeys.map(colKey => cell(cells.get(`${rowKey}|${colKey}`))).join('')}</tr>`
  ).join('\n');

  return `<!doctype html>
<html><head><meta charset="utf-8"><title>godai logo variants</title>
<style>
  body { font: 14px/1.4 system-ui, sans-serif; margin: 24px; background: #ddd; color: #222; }
  table { border-collapse: collapse; }
  th { padding: 8px 12px; font-weight: 600; text-align: left; white-space: nowrap; }
  td { padding: 8px 12px; vertical-align: top; }
  .swatch { display: flex; align-items: flex-end; gap: 10px; padding: 16px; border-radius: 8px; width: fit-content; }
  .checker { background: repeating-conic-gradient(#bbb 0% 25%, #eee 0% 50%) 0 0 / 24px 24px; }
  .name { margin: 6px 0 12px; font-size: 12px; color: #555; font-family: ui-monospace, monospace; }
  .scrub { position: sticky; top: 0; display: flex; align-items: center; gap: 12px; padding: 12px 16px; margin-bottom: 16px; background: #fff; border-radius: 8px; box-shadow: 0 1px 4px rgba(0,0,0,.2); }
  .scrub input { flex: 1; }
  .scrub button { width: 5em; }
  .scrub output { font-family: ui-monospace, monospace; white-space: nowrap; }
</style></head><body>
<h1>godai logo variants</h1>
<div class="scrub">
  <button id="play">pause</button>
  <input id="time" type="range" min="0" max="${SECONDS_PER_LOOP}" step="${1 / SAMPLES_PER_SECOND}" value="0">
  <output id="readout"></output>
</div>
<table>
<tr><th></th>${header}</tr>
${rows}
</table>
<script>
const svgs = [...document.querySelectorAll('svg.anim')];
const play = document.getElementById('play');
const time = document.getElementById('time');
const readout = document.getElementById('readout');
const LOOP = ${SECONDS_PER_LOOP};
let playing = true;
const show = t => { readout.value = 'STATIC_POSE_TIME = ' + t.toFixed(3); };
const pause = () => { playing = false; play.textContent = 'play'; svgs.forEach(s => s.pauseAnimations()); };
const resume = () => { playing = true; play.textContent = 'pause'; svgs.forEach(s => s.unpauseAnimations()); };
svgs.forEach(s => s.setCurrentTime(0)); // sync timelines
time.addEventListener('input', () => {
  pause();
  svgs.forEach(s => s.setCurrentTime(+time.value));
  show(+time.value);
});
play.addEventListener('click', () => (playing ? pause() : resume()));
(function tick() {
  if (playing && svgs.length) {
    const t = svgs[0].getCurrentTime() % LOOP;
    time.value = t;
    show(t);
  }
  requestAnimationFrame(tick);
})();
</script>
</body></html>`;
}

// ---- main ----
const outDir = process.argv[2] || '.';
fs.mkdirSync(outDir, { recursive: true });
const tracks = buildTracks();
const svgByFile = new Map();
for (const v of VARIANTS) {
  const svg = buildSvg(tracks, v);
  svgByFile.set(v.file, svg);
  fs.writeFileSync(path.join(outDir, v.file), svg);
  console.log(`wrote ${v.file} (${svg.length} bytes)`);
}
const html = buildPreviewHtml(svgByFile);
fs.writeFileSync(path.join(outDir, 'preview.html'), html);
console.log(`wrote preview.html (${html.length} bytes)`);
