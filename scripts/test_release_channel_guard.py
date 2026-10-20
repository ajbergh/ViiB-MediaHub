"""Protect the stable publication boundary across version and tag entry points."""
from pathlib import Path
import os
import subprocess
import sys
import unittest
from release_channel_guard import require_prerelease


class ReleaseChannelTests(unittest.TestCase):
    def test_stable_versions_and_tags_fail_closed(self):
        for version in ['0.1.0', '1.0.0', '2.12.3']:
            for options in [{'version': version}, {'tag': 'v' + version}, {'version': version, 'tag': 'v' + version}]:
                with self.subTest(options=options), self.assertRaisesRegex(ValueError, 'Stable releases are disabled'):
                    require_prerelease(**options)

    def test_current_and_other_prereleases_are_allowed(self):
        for version in ['1.0.0-rc7', '1.2.3-beta.1', '2.0.0-alpha', '3.0.0-dev.foo', '0.0.0-0']:
            with self.subTest(version=version):
                self.assertEqual(require_prerelease(version=version), version)
                self.assertEqual(require_prerelease(tag='v' + version), version)
                self.assertEqual(require_prerelease(version=version, tag='v' + version), version)

    def test_invalid_versions_cannot_be_misclassified_as_prereleases(self):
        for version in ['', '1.0', '1.0.0-', '1.0.0-rc..1', '01.0.0-rc1', '1.0.0-01',
                        '1.0.0+build', '1.0.0-rc7+build', 'v1.0.0-rc7', ' 1.0.0-rc7', '1.0.0-rc7\n']:
            with self.subTest(version=version), self.assertRaises(ValueError):
                require_prerelease(version=version)

    def test_invalid_or_mismatched_tags_are_rejected(self):
        for tag in ['1.0.0-rc7', 'vrelease', 'v1.0', 'refs/tags/v1.0.0-rc7']:
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                require_prerelease(tag=tag)
        with self.assertRaisesRegex(ValueError, 'does not match'):
            require_prerelease(version='1.0.0-rc7', tag='v1.0.0')

    def test_missing_input_fails_closed(self):
        with self.assertRaises(ValueError):
            require_prerelease()

    def test_signing_environment_does_not_enable_stable_publication(self):
        script = Path(__file__).with_name('release_channel_guard.py')
        result = subprocess.run([sys.executable, str(script), '--version', '1.0.0'],
                                env=dict(os.environ, WIN_CERT_PFX='test-only-certificate', WIN_CERT_PASSWORD='test-only-password'),
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 1)
        self.assertIn('Stable releases are disabled', result.stderr)
        self.assertNotIn('permitted', result.stdout)

    def test_cli_exit_status_protects_workflow_steps(self):
        script = Path(__file__).with_name('release_channel_guard.py')
        for arguments, expected in [(['--version', '1.0.0'], 1), (['--tag', 'v1.0.0'], 1),
                                    (['--version', '1.0.0-rc7', '--tag', 'v1.0.0-rc7'], 0),
                                    (['--tag', 'v1.0.0-rc7'], 0), ([], 1)]:
            with self.subTest(arguments=arguments):
                result = subprocess.run([sys.executable, str(script), *arguments], capture_output=True, text=True)
                self.assertEqual(result.returncode, expected, result.stderr)
                if expected == 1:
                    self.assertTrue(result.stderr)
                    self.assertNotIn('permitted', result.stdout)


if __name__ == '__main__':
    unittest.main()
