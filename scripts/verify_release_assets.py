#!/usr/bin/env python3
"""Require the complete release asset set and checksum every published input.

This verifies file integrity/coverage, not signatures, licensing or installation.
"""
import argparse
import hashlib
from pathlib import Path
import sys
from release_channel_guard import VERSION

MANIFEST = 'SHA256SUMS.txt'


def expected_release_assets(version):
    if not VERSION.fullmatch(version):
        raise ValueError('Invalid release version for asset filenames')
    prefix = f'ViiB-MediaHub-{version}'
    return sorted([
        'npm-sbom.cdx.json', 'go-modules.json',
        f'{prefix}-windows-x64-setup.exe',
        f'{prefix}-macos-arm64.dmg', f'{prefix}-macos-x64.dmg',
        f'{prefix}-linux-amd64.deb', f'{prefix}-linux-amd64.tar.gz',
        *[f'{prefix}-web-{target}.zip' for target in (
            'windows-x64', 'windows-arm64', 'macos-x64', 'macos-arm64', 'linux-amd64', 'linux-arm64')],
    ])


def verify_release_assets(directory, version):
    directory = Path(directory)
    expected = set(expected_release_assets(version))
    present = {entry.name for entry in directory.iterdir()}
    missing = expected - present
    unexpected = present - expected - {MANIFEST}
    if missing or unexpected:
        raise ValueError(f'Release asset set mismatch; missing={sorted(missing)}, unexpected={sorted(unexpected)}')
    manifest = directory / MANIFEST
    if manifest.is_symlink() or (manifest.exists() and not manifest.is_file()):
        raise ValueError('Checksum manifest must be a regular file')
    # Validate all paths before hashing/writing. Never follow an artifact symlink.
    for name in sorted(expected):
        path = directory / name
        if path.is_symlink() or not path.is_file() or path.stat().st_size == 0:
            raise ValueError(f'Release asset must be a nonempty regular file: {name}')
    assets = []
    for name in sorted(expected):
        path = directory / name
        digest = hashlib.sha256()
        with path.open('rb') as source:
            for block in iter(lambda: source.read(1024 * 1024), b''):
                digest.update(block)
        assets.append({'name': name, 'bytes': path.stat().st_size, 'sha256': digest.hexdigest()})
    manifest.write_text(''.join(f"{asset['sha256']}  {asset['name']}\n" for asset in assets), encoding='utf-8', newline='\n')
    return assets


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--directory', type=Path, required=True)
    parser.add_argument('--version', required=True)
    args = parser.parse_args()
    try:
        assets = verify_release_assets(args.directory, args.version)
    except (OSError, ValueError) as error:
        print(f'Release asset verification failed: {error}', file=sys.stderr)
        return 1
    print(f'Verified {len(assets)} release assets; wrote {MANIFEST}')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
