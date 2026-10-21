"""Regression checks for complete checksum coverage and fail-closed publication."""
from pathlib import Path
import tempfile
import subprocess
import sys
import unittest
from verify_release_assets import MANIFEST, expected_release_assets, verify_release_assets


class ReleaseAssetTests(unittest.TestCase):
    version = '1.0.0-rc7'

    def setUp(self):
        output = Path(__file__).resolve().parent.parent / 'output'
        output.mkdir(exist_ok=True)
        self.temporary = tempfile.TemporaryDirectory(dir=output, prefix='asset-test-')
        self.root = Path(self.temporary.name).resolve()
        self.assertTrue(self.root.is_relative_to(output.resolve()))
        self.addCleanup(self.temporary.cleanup)
        self.directory = self.root / 'assets'
        self.directory.mkdir()
        for name in expected_release_assets(self.version):
            (self.directory / name).write_bytes(b'abc')

    def test_manifest_covers_every_expected_file_and_is_repeatable(self):
        assets = verify_release_assets(self.directory, self.version)
        self.assertEqual(len(assets), 13)
        known_sha = 'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad'
        self.assertTrue(all(asset['sha256'] == known_sha and asset['bytes'] == 3 for asset in assets))
        first = (self.directory / MANIFEST).read_bytes()
        self.assertEqual(len(first.splitlines()), 13)
        names = {line.split('  ', 1)[1] for line in first.decode('utf-8').splitlines()}
        self.assertEqual(names, set(expected_release_assets(self.version)))
        verify_release_assets(self.directory, self.version)
        self.assertEqual((self.directory / MANIFEST).read_bytes(), first)

    def test_unexpected_file_cannot_be_published_without_checksum_coverage(self):
        (self.directory / 'unreviewed.txt').write_text('extra')
        with self.assertRaisesRegex(ValueError, 'unexpected=.*unreviewed'):
            verify_release_assets(self.directory, self.version)
        self.assertFalse((self.directory / MANIFEST).exists())

    def test_missing_asset_preserves_the_prior_manifest(self):
        (self.directory / MANIFEST).write_text('prior manifest')
        (self.directory / 'go-modules.json').unlink()
        with self.assertRaisesRegex(ValueError, 'missing=.*go-modules'):
            verify_release_assets(self.directory, self.version)
        self.assertEqual((self.directory / MANIFEST).read_text(), 'prior manifest')

    def test_empty_or_directory_asset_is_rejected(self):
        asset = self.directory / 'npm-sbom.cdx.json'
        asset.write_bytes(b'')
        with self.assertRaisesRegex(ValueError, 'nonempty regular file'):
            verify_release_assets(self.directory, self.version)
        asset.unlink(); asset.mkdir()
        with self.assertRaisesRegex(ValueError, 'nonempty regular file'):
            verify_release_assets(self.directory, self.version)

    def test_symlinks_cannot_escape_assets_or_redirect_the_manifest(self):
        asset = self.directory / 'go-modules.json'
        target = self.root / 'outside-assets'
        target.write_bytes(b'private fixture')
        asset.unlink()
        try:
            asset.symlink_to(target)
        except OSError:
            self.skipTest('Host does not grant symlink creation; Linux CI covers this case')
        with self.assertRaisesRegex(ValueError, 'nonempty regular file'):
            verify_release_assets(self.directory, self.version)
        self.assertEqual(target.read_bytes(), b'private fixture')
        asset.unlink(); asset.write_bytes(b'abc')
        (self.directory / MANIFEST).symlink_to(target)
        with self.assertRaisesRegex(ValueError, 'manifest must be a regular file'):
            verify_release_assets(self.directory, self.version)
        self.assertEqual(target.read_bytes(), b'private fixture')

    def test_cli_propagates_invalid_asset_set_as_failure(self):
        script = Path(__file__).with_name('verify_release_assets.py')
        command = [sys.executable, str(script), '--directory', str(self.directory), '--version', self.version]
        valid = subprocess.run(command, capture_output=True, text=True)
        self.assertEqual(valid.returncode, 0, valid.stderr)
        (self.directory / 'unexpected.txt').write_text('extra')
        invalid = subprocess.run(command, capture_output=True, text=True)
        self.assertEqual(invalid.returncode, 1)
        self.assertIn('unexpected', invalid.stderr)
        self.assertNotIn('Verified', invalid.stdout)

    def test_version_cannot_inject_a_path(self):
        for version in ['../../outside', 'v1.0.0-rc7', '1.0']:
            with self.subTest(version=version), self.assertRaises(ValueError):
                verify_release_assets(self.directory, version)


if __name__ == '__main__':
    unittest.main()
