#!/usr/bin/env python3
"""Verify every release payload against the expected GitHub workflow identity.

GitHub CLI performs cryptographic verification. Identity inputs must come from
the trusted workflow context (or a consumer's independently reviewed policy).
"""
import argparse
from pathlib import Path
import re
import subprocess
import sys

from verify_release_assets import verify_release_assets


def verify_release_provenance(directory, version, bundle, repository, source_digest, source_ref):
    if not re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', repository):
        raise ValueError('Expected owner/repository identity')
    if not re.fullmatch(r'[0-9a-fA-F]{40}', source_digest):
        raise ValueError('Expected full source commit SHA')
    if not re.fullmatch(r'refs/(heads|tags)/[^\s]+', source_ref):
        raise ValueError('Expected explicit source branch or tag ref')
    bundle = Path(bundle)
    if bundle.is_symlink() or not bundle.is_file() or bundle.stat().st_size == 0:
        raise ValueError('Expected nonempty regular attestation bundle')
    # Recheck the exact set and bytes immediately before verifying signatures.
    assets = verify_release_assets(directory, version)
    for asset in assets:
        subprocess.run([
            'gh', 'attestation', 'verify', str(Path(directory) / asset['name']),
            '--bundle', str(bundle), '--repo', repository,
            '--signer-workflow', f'{repository}/.github/workflows/release.yml',
            '--source-digest', source_digest, '--source-ref', source_ref,
            '--predicate-type', 'https://slsa.dev/provenance/v1',
            '--deny-self-hosted-runners',
        ], check=True)
    return len(assets)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--directory', type=Path, required=True)
    parser.add_argument('--version', required=True)
    parser.add_argument('--bundle', type=Path, required=True)
    parser.add_argument('--repository', required=True)
    parser.add_argument('--source-digest', required=True)
    parser.add_argument('--source-ref', required=True)
    args = parser.parse_args(argv)
    try:
        count = verify_release_provenance(**vars(args))
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f'Release provenance verification failed: {error}', file=sys.stderr)
        return 1
    print(f'Verified provenance for all {count} release payloads')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
