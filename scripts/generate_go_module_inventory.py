#!/usr/bin/env python3
"""Export the resolved Go module graph without machine-specific cache paths.

This is a dependency inventory, not an SBOM, license approval or attestation.
"""
import argparse
import json
from pathlib import Path
import subprocess


def inventory_from_stream(text):
    decoder = json.JSONDecoder()
    modules = []
    position = 0
    while position < len(text):
        if text[position].isspace():
            position += 1
            continue
        module, position = decoder.raw_decode(text, position)
        if not isinstance(module, dict) or not isinstance(module.get('Path'), str) or not module['Path']:
            raise ValueError('Go module record requires a non-empty Path')
        if module.get('Error'):
            raise ValueError('Go module graph contains an error')
        entry = {key: module[key] for key in ('Path', 'Version', 'Main', 'Indirect', 'GoVersion', 'Sum', 'GoModSum') if key in module}
        replacement = module.get('Replace')
        if replacement is not None:
            if not isinstance(replacement, dict) or replacement.get('Error'):
                raise ValueError('Invalid Go module replacement')
            # Local replacements expose checkout paths and cannot identify a
            # reproducible release input. Fail rather than silently omit them.
            if not replacement.get('Path') or not replacement.get('Version'):
                raise ValueError('Release inventory requires versioned module replacements')
            entry['Replace'] = {key: replacement[key] for key in ('Path', 'Version', 'GoVersion', 'Sum', 'GoModSum') if key in replacement}
        modules.append(entry)
    if not modules or sum(bool(module.get('Main')) for module in modules) != 1:
        raise ValueError('Go module inventory must contain exactly one main module')
    if len({module['Path'] for module in modules}) != len(modules):
        raise ValueError('Duplicate Go module paths')
    return {'schemaVersion': 1, 'modules': sorted(modules, key=lambda module: module['Path'])}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    result = subprocess.run(['go', 'list', '-mod=readonly', '-m', '-json', 'all'],
                            cwd=root / 'backend', check=True, stdout=subprocess.PIPE, text=True, encoding='utf-8')
    inventory = inventory_from_stream(result.stdout)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(inventory, indent=2, sort_keys=True) + '\n', encoding='utf-8')
    print(f"Recorded {len(inventory['modules'])} Go modules in {args.output.name}")


if __name__ == '__main__':
    main()
