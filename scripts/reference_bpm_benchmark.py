#!/usr/bin/env python3
"""Emit Phase 0 benchmark results from a development-only librosa reference.

This script is intentionally outside the ViiB application and never imports
application code. It normalizes an external reference into analysisbench's
JSON result schema so the existing Go comparison command remains authoritative
for metric calculation. It is not a production dependency or a code port.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time
from typing import Any


SAMPLE_RATE = 22_050


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True,
                        help="new result JSON path; refuses to overwrite")
    parser.add_argument("--split", choices=("tuning", "held_out"), default="tuning")
    parser.add_argument("--allow-held-out", action="store_true",
                        help="required acknowledgement before evaluating held-out audio")
    parser.add_argument("--path-base", type=Path, default=Path.cwd(),
                        help="base for relative manifest paths (default: current directory)")
    parser.add_argument("--ffmpeg", default="ffmpeg", help="FFmpeg executable")
    parser.add_argument("--start-bpm", type=float, default=120.0)
    parser.add_argument("--tightness", type=float, default=100.0)
    return parser.parse_args()


def file_hash(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1 << 20), b""):
            digest.update(block)
    return digest.hexdigest()


def ffmpeg_version(executable: str) -> str:
    completed = subprocess.run([executable, "-version"], capture_output=True, text=True, check=True)
    return completed.stdout.splitlines()[0]


def decode_mono(track_path: Path, executable: str) -> Any:
    import numpy as np

    completed = subprocess.run(
        [executable, "-nostdin", "-v", "error", "-i", os.fspath(track_path),
         "-map", "a:0", "-ac", "1", "-ar", str(SAMPLE_RATE), "-f", "f32le", "pipe:1"],
        capture_output=True,
        check=False,
    )
    if completed.returncode != 0:
        detail = completed.stderr.decode("utf-8", errors="replace").strip()
        raise RuntimeError(detail or f"ffmpeg exited {completed.returncode}")
    samples = np.frombuffer(completed.stdout, dtype="<f4")
    if samples.size == 0:
        raise RuntimeError("ffmpeg produced no PCM samples")
    return samples


def estimate_bpm(samples: Any, start_bpm: float, tightness: float) -> float | None:
    import librosa
    import numpy as np

    onset = librosa.onset.onset_strength(y=samples, sr=SAMPLE_RATE, hop_length=512, aggregate=np.median)
    tempo, beats = librosa.beat.beat_track(
        onset_envelope=onset,
        sr=SAMPLE_RATE,
        hop_length=512,
        start_bpm=start_bpm,
        tightness=tightness,
        trim=True,
    )
    values = np.asarray(tempo).reshape(-1)
    if values.size == 0 or np.asarray(beats).size == 0 or not np.isfinite(values[0]) or values[0] <= 0:
        return None
    return float(values[0])


def resolve_path(value: str, path_base: Path) -> Path:
    # Corpus manifests are produced on Windows, while reference measurements
    # also run on macOS and Linux. Relative paths remain local evidence, but
    # their separators must not make them Windows-only.
    candidate = Path(value.replace("\\", "/"))
    if candidate.is_absolute():
        return candidate
    return path_base / candidate


def main() -> int:
    args = parse_args()
    run_started = time.monotonic()
    if args.split == "held_out" and not args.allow_held_out:
        raise SystemExit("refusing held-out evaluation: pass --allow-held-out after freezing a tuning-selected configuration")
    if args.out.exists():
        raise SystemExit(f"refusing to overwrite existing output: {args.out}")
    if not args.manifest.is_file():
        raise SystemExit(f"manifest does not exist: {args.manifest}")

    try:
        import librosa
    except ImportError as error:
        raise SystemExit("librosa is required; create the isolated environment described in docs/REFERENCE_BPM_BENCHMARK.md") from error

    try:
        decoder_version = ffmpeg_version(args.ffmpeg)
    except (OSError, subprocess.CalledProcessError) as error:
        raise SystemExit(f"FFmpeg is required: {error}") from error

    manifest = json.loads(args.manifest.read_text(encoding="utf-8"))
    path_base = args.path_base.resolve()
    result_rows: list[dict[str, Any]] = []
    for track in manifest.get("tracks", []):
        if track.get("split") != args.split:
            continue
        row: dict[str, Any] = {"id": track["id"]}
        source_path = resolve_path(track["path"], path_base)
        try:
            bpm = estimate_bpm(decode_mono(source_path, args.ffmpeg), args.start_bpm, args.tightness)
            row["status"] = "partial"
            if bpm is None:
                row["status"] = "unknown"
            else:
                row["bpm"] = bpm
        except OSError as error:
            row.update(status="failed", error="reference_source_unavailable", errorMessage=str(error))
        except RuntimeError as error:
            row.update(status="failed", error="reference_decode_failed", errorMessage=str(error))
        except Exception as error:  # Preserve every reference-tool failure as evidence.
            row.update(status="failed", error="reference_analysis_failed", errorMessage=f"{type(error).__name__}: {error}")
        result_rows.append(row)

    if not result_rows:
        raise SystemExit(f"manifest has no {args.split!r} tracks")
    result_set = {
        "algorithm": f"librosa-{librosa.__version__}-beat-track",
        "configuration": {
            "tool": "librosa.beat.beat_track",
            "toolVersion": librosa.__version__,
            "split": args.split,
            "manifest": args.manifest.name,
            "manifestSHA256": file_hash(args.manifest),
            "pathBase": os.fspath(path_base),
            "decoder": decoder_version,
            "decoderCommand": "ffmpeg -nostdin -v error -i INPUT -map a:0 -ac 1 -ar 22050 -f f32le pipe:1",
            "sampleRate": SAMPLE_RATE,
            "hopLength": 512,
            "startBPM": args.start_bpm,
            "tightness": args.tightness,
            "audioDurationPolicy": "full track",
            "wallSeconds": round(time.monotonic() - run_started, 6),
            "purpose": "development-only external reference; not shipped or used by ViiB production analysis",
        },
        "results": result_rows,
    }
    with args.out.open("x", encoding="utf-8", newline="\n") as destination:
        json.dump(result_set, destination, indent=2)
        destination.write("\n")
    print(json.dumps({"out": os.fspath(args.out), "tracks": len(result_rows), "split": args.split}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
