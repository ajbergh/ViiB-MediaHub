#!/usr/bin/env python3
"""Check repository Markdown local link targets and documentation JSON syntax.

External URL availability and generated Markdown anchor semantics are excluded.
Run from any directory: python scripts/check_documentation_links.py
"""
import json
from pathlib import Path
import re
import subprocess
from urllib.parse import unquote, urlsplit


def main():
    root = Path(__file__).resolve().parent.parent
    names = subprocess.check_output(
        ['git', '-c', f'safe.directory={root.as_posix()}', 'ls-files', '--cached', '--others', '--exclude-standard'],
        cwd=root, text=True,
    ).splitlines()
    failures = []
    documents = links = artifacts = 0
    inline = re.compile(r'!?\[[^\]\n]*\]\((<[^>\n]*>|[^\s)]+)(?:\s+[\'\"][^\n]*?[\'\"])?\)')
    definition = re.compile(r'^\s*\[[^\]\n]+\]:\s*(<[^>\n]*>|\S+)', re.MULTILINE)
    for name in sorted(set(names)):
        path = root / name
        if not path.is_file():
            continue
        if path.suffix == '.md':
            documents += 1
            text = path.read_text(encoding='utf-8-sig')
            text = re.sub(r'^```[^\n]*\n.*?^```[^\n]*$', '', text, flags=re.MULTILINE | re.DOTALL)
            for match in list(inline.finditer(text)) + list(definition.finditer(text)):
                target = match[1].strip('<>')
                parsed = urlsplit(target)
                if parsed.scheme or parsed.netloc or not parsed.path:
                    continue
                links += 1
                destination = (root / unquote(parsed.path).lstrip('/')) if parsed.path.startswith('/') else path.parent / unquote(parsed.path)
                if not destination.exists():
                    failures.append(f'{name}: missing target {target}')
        elif name.startswith('docs/') and path.suffix == '.json':
            artifacts += 1
            try:
                json.loads(path.read_text(encoding='utf-8-sig'))
            except (ValueError, UnicodeError) as error:
                failures.append(f'{name}: invalid JSON: {error}')
    print(json.dumps({'markdownFiles': documents, 'localLinks': links, 'documentationJSONFiles': artifacts, 'failures': failures}, indent=2))
    return bool(failures)


if __name__ == '__main__':
    raise SystemExit(main())
