import fs from 'node:fs/promises';
import path from 'node:path';

const PROJECT_ROOT = process.cwd();
const BASELINE_PATH = path.join(PROJECT_ROOT, 'scripts', 'raw-color-baseline.json');

const EXCLUDED_DIR_NAMES = new Set([
  'node_modules',
  'dist',
  'build',
  'backend',
  '.git',
]);

const HEX_REGEX = /#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})\b/g;

function toPosixPath(relativePath) {
  return relativePath.split(path.sep).join('/');
}

async function* walk(dir) {
  const entries = await fs.readdir(dir, { withFileTypes: true });
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (EXCLUDED_DIR_NAMES.has(entry.name)) continue;
      yield* walk(fullPath);
      continue;
    }
    yield fullPath;
  }
}

function getLineNumberForIndex(text, index) {
  let line = 1;
  for (let i = 0; i < index; i++) {
    if (text.charCodeAt(i) === 10) line++;
  }
  return line;
}

async function loadBaseline() {
  try {
    return JSON.parse(await fs.readFile(BASELINE_PATH, 'utf8'));
  } catch (error) {
    if (error?.code === 'ENOENT') return {};
    throw error;
  }
}

async function main() {
  const baseline = await loadBaseline();
  const violations = [];
  const counts = new Map();

  for await (const filePath of walk(PROJECT_ROOT)) {
    if (!filePath.endsWith('.tsx')) continue;

    const relPosix = toPosixPath(path.relative(PROJECT_ROOT, filePath));
    const content = await fs.readFile(filePath, 'utf8');

    let match;
    while ((match = HEX_REGEX.exec(content)) !== null) {
      const line = getLineNumberForIndex(content, match.index);
      violations.push({ file: relPosix, line, value: match[0] });
      counts.set(relPosix, (counts.get(relPosix) || 0) + 1);
    }
  }

  const regressions = [];
  for (const [file, count] of counts) {
    const allowed = Number(baseline[file] || 0);
    if (count > allowed) regressions.push({ file, count, allowed });
  }

  if (regressions.length > 0) {
    console.error('Raw-color baseline regressed. Replace new literals with design tokens:');
    for (const regression of regressions) {
      console.error(`- ${regression.file}: ${regression.count} literal(s), baseline ${regression.allowed}`);
      for (const violation of violations.filter((item) => item.file === regression.file)) {
        console.error(`    L${violation.line}: ${violation.value}`);
      }
    }
    process.exitCode = 1;
    return;
  }

  const remaining = violations.length;
  if (remaining > 0) {
    console.warn(`Baseline accepted: ${remaining} legacy raw-color literal(s) remain across ${counts.size} file(s).`);
    console.warn('Do not increase these counts; reduce scripts/raw-color-baseline.json as files are tokenized.');
  } else {
    console.log('OK: no raw hex colors found in TSX.');
  }
}

await main();
