/** Finds style violations in source and compares their counts with the configured baseline. */

import fs from 'node:fs/promises';
import path from 'node:path';
const excluded = new Set(['node_modules', 'dist', 'build', 'backend', '.git', '.worktrees', '.wails', '.reference-bpm-venv', '.venv', 'venv', '.codex', '.agents', '.aws', '.tmp-review']);
export async function checkStyleRatchet({ name, baselineFile, extensions, pattern }) {
 const root = process.cwd();
 const baselinePath = path.join(root, 'scripts', baselineFile);
 const baseline = JSON.parse(await fs.readFile(baselinePath, 'utf8'));
 const counts = {};
 async function walk(dir) {
  for (const entry of await fs.readdir(dir, { withFileTypes: true })) {
   const full = path.join(dir, entry.name);
   if (entry.isDirectory()) { if (!excluded.has(entry.name)) await walk(full); continue; }
   if (!entry.isFile() || !extensions.includes(path.extname(entry.name))) continue;
   const content = await fs.readFile(full, 'utf8');
   const count = [...content.matchAll(pattern)].length;
   if (count) counts[path.relative(root, full).split(path.sep).join('/')] = count;
  }
 }
 await walk(root);
 const regressions = Object.entries(counts).filter(([file,count]) => count > (baseline[file] || 0));
 if (regressions.length) {
  console.error(`${name} regressed:`);
  for (const [file,count] of regressions) console.error(`  ${file}: ${count}, allowed ${baseline[file] || 0}`);
  process.exitCode = 1; return;
 }
 const stale = Object.entries(baseline).filter(([file,count]) => (counts[file] || 0) < count);
 if (stale.length) {
  if (process.argv.includes('--tighten')) {
   await fs.writeFile(baselinePath, JSON.stringify(counts, null, 2)+'\n');
  } else {
   console.error(`${name}: baseline must tighten for ${stale.map(([file])=>file).join(', ')}. Run with --tighten.`);
   process.exitCode = 1;return;
  }
 }
 console.log(`${name}: ${Object.values(counts).reduce((a,b)=>a+b,0)} legacy occurrences; no regressions.`);
}
