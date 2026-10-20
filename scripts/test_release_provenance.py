"""Test publication policy wiring; mocks are not cryptographic qualification."""
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from verify_release_assets import expected_release_assets
from verify_release_provenance import main, verify_release_provenance


class ReleaseProvenanceTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.directory = self.root / 'assets'
        self.directory.mkdir()
        for name in expected_release_assets('1.0.0-rc7'):
            (self.directory / name).write_bytes(b'candidate payload')
        self.bundle = self.root / 'bundle.jsonl'
        self.bundle.write_text('synthetic bundle; not a signature\n', encoding='utf-8')
        self.policy = dict(directory=self.directory, version='1.0.0-rc7',
                           bundle=self.bundle, repository='ajbergh/ViiB-MediaHub',
                           source_digest='a' * 40, source_ref='refs/heads/main')

    @patch('verify_release_provenance.subprocess.run')
    def test_every_payload_requires_exact_trusted_identity(self, run):
        self.assertEqual(verify_release_provenance(**self.policy), 13)
        self.assertEqual(run.call_count, 13)
        subjects = set()
        for call in run.call_args_list:
            command = call.args[0]
            subjects.add(Path(command[3]).name)
            self.assertEqual(command[:3], ['gh', 'attestation', 'verify'])
            for flag, value in {
                '--bundle': str(self.bundle), '--repo': self.policy['repository'],
                '--signer-workflow': 'ajbergh/ViiB-MediaHub/.github/workflows/release.yml',
                '--source-digest': 'a' * 40, '--source-ref': 'refs/heads/main',
                '--predicate-type': 'https://slsa.dev/provenance/v1',
            }.items():
                self.assertEqual(command[command.index(flag) + 1], value)
            self.assertIn('--deny-self-hosted-runners', command)
            self.assertTrue(call.kwargs['check'])
        self.assertEqual(subjects, set(expected_release_assets('1.0.0-rc7')))

    @patch('verify_release_provenance.subprocess.run')
    def test_verification_failure_stops_before_remaining_subjects(self, run):
        run.side_effect = [None, None, subprocess.CalledProcessError(1, 'gh')]
        with self.assertRaises(subprocess.CalledProcessError):
            verify_release_provenance(**self.policy)
        self.assertEqual(run.call_count, 3)

    @patch('verify_release_provenance.subprocess.run')
    def test_invalid_policy_fails_before_cli(self, run):
        for key, value in [('repository', 'owner'), ('source_digest', 'main'),
                           ('source_ref', 'main'), ('source_ref', 'refs/pull/1/merge')]:
            with self.subTest(key=key, value=value):
                with self.assertRaises(ValueError):
                    verify_release_provenance(**dict(self.policy, **{key: value}))
        run.assert_not_called()

    @patch('verify_release_provenance.subprocess.run')
    def test_missing_or_empty_bundle_fails_before_cli(self, run):
        self.bundle.unlink()
        with self.assertRaises(ValueError):
            verify_release_provenance(**self.policy)
        self.bundle.touch()
        with self.assertRaises(ValueError):
            verify_release_provenance(**self.policy)
        run.assert_not_called()

    @patch('verify_release_provenance.subprocess.run')
    def test_unexpected_asset_fails_before_cli(self, run):
        (self.directory / 'unreviewed.zip').write_bytes(b'extra')
        with self.assertRaises(ValueError):
            verify_release_provenance(**self.policy)
        run.assert_not_called()

    @patch('verify_release_provenance.subprocess.run')
    def test_cli_fails_closed_when_gh_fails_or_is_missing(self, run):
        arguments = []
        for key, value in self.policy.items():
            arguments.extend(['--' + key.replace('_', '-'), str(value)])
        for error in [subprocess.CalledProcessError(1, 'gh'), FileNotFoundError('gh')]:
            run.side_effect = error
            with self.subTest(error=type(error).__name__):
                self.assertEqual(main(arguments), 1)


if __name__ == '__main__':
    unittest.main()
