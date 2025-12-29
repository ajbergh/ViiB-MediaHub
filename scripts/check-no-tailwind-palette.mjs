import fs from 'node:fs/promises';
import path from 'node:path';

const STRICT = process.argv.includes('--strict');

const ROOT = process.cwd();
const EXCLUDED_DIRS = new Set([
  'node_modules',
  'dist',
  'build',
  '.git',
  'backend',
  '.wails',
]);

const INCLUDED_EXTS = new Set(['.ts', '.tsx', '.js', '.jsx']);

const PALETTE_REGEX = /\b(?:bg|text|border|ring|stroke|fill|from|to|via|shadow)-(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d{2,3}(?:\/\d{1,3})?\b/g;

async function* walk(dir) {
  const entries = await fs.readdir(dir, { withFileTypes: true });
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (EXCLUDED_DIRS.has(entry.name)) continue;
      yield* walk(fullPath);
      continue;
    }

    if (!entry.isFile()) continue;
    if (!INCLUDED_EXTS.has(path.extname(entry.name))) continue;
    yield fullPath;
  }
}

function findMatches(text) {
  const lines = text.split(/\r?\n/);
  const hits = [];

  for (let idx = 0; idx < lines.length; idx += 1) {
    const line = lines[idx];
    PALETTE_REGEX.lastIndex = 0;
    const matches = [...line.matchAll(PALETTE_REGEX)].map((m) => m[0]);
    if (matches.length === 0) continue;

    hits.push({
      lineNumber: idx + 1,
      classes: Array.from(new Set(matches)).sort(),
      line: line.trim(),
    });
  }

  return hits;
}

async function main() {
  const results = [];

  for await (const filePath of walk(ROOT)) {
    const rel = path.relative(ROOT, filePath);
    const text = await fs.readFile(filePath, 'utf8');
    const hits = findMatches(text);
    if (hits.length === 0) continue;
    results.push({ file: rel, hits });
  }

  if (results.length === 0) {
    console.log('✅ No default Tailwind palette classes found.');
    process.exit(0);
  }

  console.log(`⚠️ Found ${results.length} file(s) using default Tailwind palette classes (non-token).`);
  console.log('   (Use --strict to fail with exit code 1)\n');

  for (const r of results) {
    console.log(r.file);
    for (const h of r.hits) {
      console.log(`  L${h.lineNumber}: ${h.classes.join(' ')}`);
    }
    console.log('');
  }

  process.exit(STRICT ? 1 : 0);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
