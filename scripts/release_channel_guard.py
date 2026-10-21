#!/usr/bin/env python3
"""Permit prerelease packaging only until stable signing is qualified."""
import argparse
import re
import sys

VERSION = re.compile(r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?')
STABLE_DISABLED = ('Stable releases are disabled until production signing, macOS notarization '
                   'and installer verification are implemented and qualified. Use a prerelease '
                   'version for unqualified packaging; signing secrets do not override this gate.')


def require_prerelease(version=None, tag=None):
    if tag is not None:
        if not tag.startswith('v'):
            raise ValueError('Release tags must begin with v')
        if version is None:
            version = tag[1:]
        elif tag != 'v' + version:
            raise ValueError('Release tag does not match the resolved version')
    if not version:
        raise ValueError('A release version or tag is required')
    match = VERSION.fullmatch(version)
    if not match:
        raise ValueError('Invalid release version (build metadata is not supported)')
    prerelease = match.group(4)
    if prerelease is None:
        raise ValueError(STABLE_DISABLED)
    if any(identifier.isdigit() and len(identifier) > 1 and identifier.startswith('0')
           for identifier in prerelease.split('.')):
        raise ValueError('Numeric prerelease identifiers must not have leading zeroes')
    return version


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version')
    parser.add_argument('--tag')
    args = parser.parse_args()
    try:
        version = require_prerelease(args.version, args.tag)
    except ValueError as error:
        print(error, file=sys.stderr)
        return 1
    print(f'Prerelease channel permitted: {version}')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
