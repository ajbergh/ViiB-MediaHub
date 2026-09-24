"""Focused tests for the development-only rhythm benchmark contract."""

from __future__ import annotations

import json
import hashlib
from pathlib import Path
import sys
import tempfile
import unittest
import wave

sys.path.insert(0, str(Path(__file__).resolve().parent))

from rhythm_benchmark_common import (  # noqa: E402
    apply_overlap_audit, evidence_class, normalized_row, persist_downbeat_provenance,
    validate_canonical_wav,
)


class RhythmBenchmarkContractTests(unittest.TestCase):
    def test_normalized_row_keeps_measured_downbeats_and_timestamps(self) -> None:
        row = normalized_row("track-1", status="partial", bpm=120,
                             beats=[0.0, 0.5, 1.0], downbeats=[0.0, 2.0])
        self.assertEqual(row["rhythm"]["beats"][1]["timeSeconds"], 0.5)
        self.assertEqual(row["rhythm"]["downbeats"][0]["provenance"], "measured")
        self.assertEqual(row["rhythm"]["downbeats"][0]["provenanceScope"], "benchmark-reference-prediction")
        self.assertEqual(row["rhythm"]["downbeatProvenance"], "measured")
        self.assertEqual(row["bpm"], 120)
        manual = normalized_row("track-1", status="partial", downbeats=[0], downbeat_provenance="manual")
        derived = normalized_row("track-1", status="partial", downbeats=[0], downbeat_provenance="inferred-from-meter")
        self.assertEqual(manual["rhythm"]["downbeatProvenance"], "manual")
        self.assertEqual(derived["rhythm"]["downbeatProvenance"], "inferred-from-meter")

    def test_provenance_values_are_distinct_and_mutually_exclusive(self) -> None:
        self.assertEqual(persist_downbeat_provenance(measured=True), "measured")
        self.assertEqual(persist_downbeat_provenance(manual=True), "manual")
        self.assertEqual(persist_downbeat_provenance(meter_derived=True), "inferred-from-meter")
        with self.assertRaises(ValueError):
            persist_downbeat_provenance(measured=True, meter_derived=True)

    def test_canonical_wav_validates_pcm_geometry_and_vii_b_metadata(self) -> None:
        with tempfile.TemporaryDirectory() as temp_name:
            root = Path(temp_name)
            path = root / "canonical.wav"
            with wave.open(str(path), "wb") as output:
                output.setnchannels(1)
                output.setsampwidth(2)
                output.setframerate(8000)
                output.writeframes(b"\x00\x00" * 8)
            track = {
                "canonicalWavPath": "canonical.wav",
                "canonicalAudio": {
                    "decoder": "ViiB", "decoderVersion": "go-mp3-v0.3.4",
                    "encoding": "pcm_s16le", "channels": 1, "sampleRate": 8000, "frames": 8,
                    "sourceAudioSHA256": hashlib.sha256(b"\x00\x00" * 8).hexdigest(), "timelineOriginSeconds": 0,
                    "resampleDelaySamples": 0, "startTrimSamples": 0,
                },
            }
            resolved, timing = validate_canonical_wav(track, root)
            self.assertEqual(resolved, path)
            self.assertEqual(timing["durationSeconds"], 0.001)
            self.assertIn("ViiB canonical decoded source frame 0", timing["timestampOrigin"])

    def test_canonical_wav_fails_closed_without_metadata(self) -> None:
        with self.assertRaisesRegex(ValueError, "canonicalWavPath and canonicalAudio"):
            validate_canonical_wav({"path": "original.mp3"}, Path.cwd())

    def test_overlap_audit_overlay_keeps_contamination_separate_from_split(self) -> None:
        with tempfile.TemporaryDirectory() as temp_name:
            root = Path(temp_name)
            manifest_path = root / "manifest.json"
            manifest = {"tracks": [{"id": "a", "split": "held_out"}, {"id": "b", "split": "tuning"}]}
            manifest_path.write_text(json.dumps(manifest), encoding="utf-8")
            audit_path = root / "audit.json"
            audit_path.write_text(json.dumps({
                "schema": "viib.training-overlap-audit.v1",
                "manifestSHA256": hashlib.sha256(manifest_path.read_bytes()).hexdigest(),
                "tracks": [
                    {"id": "a", "classification": "training-overlap-contaminated"},
                    {"id": "b", "classification": "overlap-reviewed-clear"},
                ],
            }), encoding="utf-8")
            audit_hash = apply_overlap_audit(manifest, manifest_path, audit_path)
            self.assertTrue(audit_hash)
            self.assertEqual(manifest["tracks"][0]["split"], "held_out")
            self.assertEqual(evidence_class(manifest["tracks"][0]), "training-overlap-contaminated")
            self.assertEqual(evidence_class(manifest["tracks"][1]), "overlap-reviewed-clear")


if __name__ == "__main__":
    unittest.main()
