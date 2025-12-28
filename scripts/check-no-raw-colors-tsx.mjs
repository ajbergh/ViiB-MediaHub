import fs from 'node:fs/promises';
import path from 'node:path';

const PROJECT_ROOT = process.cwd();

const EXCLUDED_DIR_NAMES = new Set([
  'node_modules',
  'dist',
  'build',
  'backend',
  '.git',
]);

// Paths are relative to repo root using forward slashes.
const ALLOWLIST = new Set([
  // Add exact file paths here if a TSX file must contain literal colors.
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
  // 1-based line number
  let line = 1;
  for (let i = 0; i < index; i++) {
    if (text.charCodeAt(i) === 10) line++;
  }
  return line;
}

async function main() {
  const violations = [];

  for await (const filePath of walk(PROJECT_ROOT)) {
    if (!filePath.endsWith('.tsx')) continue;

    const rel = path.relative(PROJECT_ROOT, filePath);
    const relPosix = toPosixPath(rel);
    if (ALLOWLIST.has(relPosix)) continue;

    const content = await fs.readFile(filePath, 'utf8');

    let match;
    while ((match = HEX_REGEX.exec(content)) !== null) {
      const line = getLineNumberForIndex(content, match.index);
      violations.push({ file: relPosix, line, value: match[0] });
    }
  }

  if (violations.length > 0) {
    // Keep output readable in CI.
    console.error('Found raw hex colors in TSX (use Tailwind tokens or VIIB_COLOR_VALUES instead):');
    for (const v of violations) {
      console.error(`- ${v.file}:${v.line}  ${v.value}`);
    }
    process.exitCode = 1;
    return;
  }

  console.log('OK: no raw hex colors found in TSX.');
}

await main();
