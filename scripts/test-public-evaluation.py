import importlib.util
import io
from pathlib import Path
import random
import unittest
from unittest.mock import patch
import zipfile

from public_zip_ranges import PublicZipRanges


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


corpus = load("corpus", "import-winterreise-evaluation.py")
boundaries = load("boundaries", "evaluate-structure-boundaries.py")
guitarset = load("guitarset", "import-guitarset-evaluation.py")
beat_grid = load("beat_grid", "evaluate-beat-grid.py")
fsld = load("fsld", "select-fsld-evaluation.py")
fsld_import = load("fsld_import", "import-fsld-evaluation.py")


class PublicEvaluationTests(unittest.TestCase):
    def test_fsld_duplicate_fence_excludes_both_splits_without_moving_labels(self):
        rows = [{"id": "fsld-1", "split": "tuning", "expectedBpm": 120, "license": "CC0", "recordingGroup": "uploader-1", "agreedKeyMode": ["none", "none"]},
                {"id": "fsld-2", "split": "held_out", "expectedBpm": 120, "license": "CC0", "recordingGroup": "uploader-2", "agreedKeyMode": ["none", "none"]}]
        selection = {"dataset": "test", "recordings": rows}
        acquired = {row["id"]: {"available": True, "sha256": "identical-bytes", "format": {"supported": True}} for row in rows}
        paths = {row["id"]: Path(row["id"] + ".wav") for row in rows}
        manifest, exclusions = fsld_import.build_manifest(selection, acquired, paths)
        self.assertEqual(manifest["tracks"], [])
        self.assertEqual(exclusions["identical-audio-across-splits"], 2)
        rows[1]["split"] = "tuning"
        manifest, exclusions = fsld_import.build_manifest(selection, acquired, paths)
        self.assertEqual(len(manifest["tracks"]), 1)
        self.assertEqual(manifest["tracks"][0]["id"], "fsld-1")
        self.assertTrue(manifest["tracks"][0]["expectedKeyUnknown"])
        self.assertEqual(manifest["tracks"][0]["expectedBpm"], 120)
        self.assertEqual(exclusions["duplicate-audio-within-split"], 1)
        rows[1]["expectedBpm"] = 121
        manifest, exclusions = fsld_import.build_manifest(selection, acquired, paths)
        self.assertEqual(manifest["tracks"], [])
        self.assertEqual(exclusions["duplicate-audio-conflicting-labels"], 2)

    def test_public_ranges_never_download_ignored_range_body(self):
        class Response(io.BytesIO):
            status = 200
            headers = {}
            read_called = False
            def read(self, *args):
                self.read_called = True
                return super().read(*args)
        response = Response(b"unbounded body")
        with patch("urllib.request.urlopen", return_value=response):
            with self.assertRaises(ValueError):
                PublicZipRanges("https://example.test/public.zip", 10**10).read(22)
        self.assertFalse(response.read_called)

    def test_public_ranges_decode_zip_and_bind_exact_release_names(self):
        storage = io.BytesIO()
        with zipfile.ZipFile(storage, "w", zipfile.ZIP_DEFLATED) as package:
            package.writestr("audio/wav/123_456.wav.wav", b"public fixture bytes")
        archive = storage.getvalue()
        class Response(io.BytesIO):
            status = 206
        def open_range(request, timeout):
            start, end = map(int, request.headers["Range"].removeprefix("bytes=").split("-"))
            response = Response(archive[start:end + 1])
            response.headers = {"Content-Range": f"bytes {start}-{end}/{len(archive)}"}
            return response
        with patch("urllib.request.urlopen", side_effect=open_range):
            source = PublicZipRanges("https://example.test/public.zip", len(archive))
            with zipfile.ZipFile(source) as package:
                self.assertEqual(package.read("audio/wav/123_456.wav.wav"), b"public fixture bytes")
        self.assertEqual(fsld_import.member_identity("audio/wav/123_456.mp3.wav"), "123")
        self.assertIsNone(fsld_import.member_identity("audio/wav/0123_other.wav"))
        self.assertIsNone(fsld_import.member_identity("../../123_456.wav.wav"))

    def test_public_ranges_retry_truncation_without_accepting_partial_bytes(self):
        class Response(io.BytesIO):
            status = 206
            headers = {"Content-Range": "bytes 0-3/4"}
        with patch("urllib.request.urlopen", side_effect=[Response(b"ab"), Response(b"abcd")]) as request, patch("time.sleep"):
            source = PublicZipRanges("https://example.test/public.zip", 4)
            self.assertEqual(source.read(4), b"abcd")
            self.assertEqual(source.transferred, 4)
            self.assertEqual(request.call_count, 2)

    def test_fsld_requires_unanimous_labels_and_per_sound_license(self):
        row = {"bpm": "120", "defined_tempo": True, "well_cut": True, "key": "c", "mode": "maj", "genres": ["electronic"]}
        metadata = {"1": {"id": 1, "username": "public-corpus-uploader", "license": "http://creativecommons.org/licenses/by/3.0/"},
                    "2": {"id": 2, "username": "public-corpus-uploader", "license": "http://creativecommons.org/licenses/by/3.0/"}}
        selected, reasons = fsld.select({"1": [row, row], "2": [row, row]}, metadata)
        self.assertEqual((len(selected), reasons), (2, {}))
        self.assertEqual(selected[0]["split"], selected[1]["split"])
        self.assertEqual(selected[0]["recordingGroup"], selected[1]["recordingGroup"])
        for changed in ({**row, "bpm": "60"}, {**row, "discard": True}, {**row, "save_for_later": True}):
            self.assertEqual(fsld.select({"1": [row, row, changed]}, metadata)[0], [])
        metadata["1"]["license"] = "http://creativecommons.org/licenses/by-nc/3.0/"
        self.assertEqual(fsld.select({"1": [row, row]}, metadata)[0], [])

    def test_beat_scorer_counts_missing_and_rejects_split_leakage(self):
        reference = {"dataset": "test", "tracks": [
            {"id": "one", "split": "tuning", "duration": 2, "beats": [0, 0.5, 1, 1.5]},
            {"id": "reserved", "split": "held_out", "duration": 2, "beats": [0, 0.5, 1, 1.5]}]}
        results = {"algorithm": "test", "results": [{"id": "one", "beatPositions": [0.04, 0.54, 1.04, 1.54, 2.04]}]}
        report = beat_grid.evaluate(reference, results, "tuning")
        self.assertEqual((report["matched"], report["micro"]["f1"], report["tracks"][0]["excludedAfterAnnotatedSpan"]), (4, 1, 1))
        missing = beat_grid.evaluate(reference, {"algorithm": "test", "results": []}, "tuning")
        self.assertEqual((missing["matched"], missing["missingBeatTracks"], missing["micro"]["f1"]), (0, 1, 0))
        results["results"][0]["beatPositions"] = [0.2, 0.7, 1.2, 1.7]
        self.assertEqual(beat_grid.evaluate(reference, results, "tuning")["matched"], 0)
        with self.assertRaises(ValueError):
            beat_grid.evaluate(reference, results, "held_out")

    def test_guitarset_split_keeps_related_versions_together(self):
        for style in ("BN", "Funk", "Jazz", "Rock", "SS"):
            for player in range(6):
                for part in ("comp", "solo"):
                    for tempo, tonic in ((90, "C#"), (130, "A")):
                        self.assertEqual(guitarset.identity(f"0{player}_{style}3-{tempo}-{tonic}_{part}")["split"], "held_out")
                        self.assertEqual(guitarset.identity(f"0{player}_{style}1-{tempo}-{tonic}_{part}")["split"], "tuning")

    def test_guitarset_uses_jams_labels_and_checks_grid(self):
        stem = "00_Rock1-120-C_comp"
        document = {"file_metadata": {"title": stem, "duration": 2}, "annotations": [
            {"namespace": "tempo", "data": [{"value": 120}]},
            {"namespace": "key_mode", "data": [{"value": "C:major"}]},
            {"namespace": "beat_position", "data": [
                {"time": i / 2, "value": {"position": i + 1, "num_beats": 4, "beat_units": 4}} for i in range(4)]}]}
        info, bpm, key, grid = guitarset.labels(document, stem)
        self.assertEqual((bpm, key, grid["downbeats"], grid["meters"]), (120, "C major", [0], [[4, 4]]))
        document["annotations"][2]["data"][1]["time"] = 0.6
        with self.assertRaises(ValueError):
            guitarset.labels(document, stem)
        document["annotations"].append({"namespace": "tempo", "data": [{"value": 120}]})
        with self.assertRaises(ValueError):
            guitarset.labels(document, stem)

    def test_performance_specific_labels_and_transposition(self):
        data = 'WorkID;PerformanceID;key\r\n'
        for number in range(1, 25):
            data += f'Schubert_D911-{number:02d};SC06;D:maj\r\n'
            data += f'Schubert_D911-{number:02d};HU33;C:min\r\n'
        keys = corpus.global_keys(data.encode())
        self.assertEqual(len(keys), 24)
        self.assertEqual(set(keys.values()), {"C minor"})
        with self.assertRaises(ValueError):
            corpus.global_keys((data + 'Schubert_D911-01;HU33;D:maj\n').encode())

    def test_structure_excludes_annotation_endpoints(self):
        reference = corpus.structure_reference(b'start;end;structure\n1;3;I\n3;8;A\n8;10;B\n')
        self.assertEqual(reference["boundaries"], [3, 8])
        self.assertEqual((reference["annotatedStart"], reference["annotatedEnd"]), (1, 10))
        with self.assertRaises(ValueError):
            corpus.structure_reference(b'start;end;structure\n1;4;I\n3;8;A\n')

    def test_matches_are_one_to_one_and_maximal(self):
        self.assertEqual(boundaries.match_count([1], [0.8, 1, 1.2], 0.5), 1)
        self.assertEqual(boundaries.match_count([1, 2], [1.5], 0.5), 1)
        rng = random.Random(71)
        for _ in range(100):
            ref, pred = sorted(rng.sample(range(20), 5)), sorted(rng.sample(range(20), 6))
            dp = [[0] * (len(pred) + 1) for _ in range(len(ref) + 1)]
            for i, a in enumerate(ref, 1):
                for j, b in enumerate(pred, 1):
                    dp[i][j] = max(dp[i-1][j], dp[i][j-1], dp[i-1][j-1] + int(abs(a-b) <= 2))
            self.assertEqual(boundaries.match_count(ref, pred, 2), dp[-1][-1])

    def test_missing_structure_counts_as_missed_reference(self):
        reference = {"dataset": "test", "tracks": {"one": {"annotatedStart": 1, "annotatedEnd": 10, "boundaries": [3, 8]}}}
        report = boundaries.evaluate(reference, {"algorithm": "test", "results": []})
        self.assertEqual(report["windows"][0]["missingStructureTracks"], 1)
        self.assertEqual(report["windows"][0]["micro"]["f1"], 0)
        with self.assertRaises(ValueError):
            boundaries.match_count([2, 1], [1], 0.5)


if __name__ == "__main__":
    unittest.main()
