#!/usr/bin/env python3
"""Emit benchmark-only Essentia RhythmExtractor2013 BPM and beat ticks.

Run this only in an isolated development environment. Essentia is an AGPL
reference process and is never imported, linked, or shipped by ViiB.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import time
from typing import Any


MULTIFEATURE_CONFIDENCE_MAX = 5.32


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
    parser.add_argument("--method", choices=("multifeature", "degara"), default="multifeature")
    parser.add_argument("--sample-rate", type=int, default=44_100)
    parser.add_argument("--min-tempo", type=int, default=40)
    parser.add_argument("--max-tempo", type=int, default=208)
    return parser.parse_args()


def file_hash(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1 << 20), b""):
            digest.update(block)
    return digest.hexdigest()


def resolve_path(value: str, path_base: Path) -> Path:
    candidate = Path(value.replace("\\", "/"))
    return candidate if candidate.is_absolute() else path_base / candidate


def finite_ticks(ticks: Any) -> list[float]:
    values = [float(tick) for tick in ticks]
    if any(not math.isfinite(tick) or tick < 0 for tick in values):
        raise RuntimeError("Essentia returned an invalid beat tick")
    if any(right <= left for left, right in zip(values, values[1:])):
        raise RuntimeError("Essentia returned non-ascending beat ticks")
    return values


def analyze(path: Path, method: str, sample_rate: int, min_tempo: int, max_tempo: int) -> tuple[float, list[float], float]:
    import essentia.standard as es

    samples = es.MonoLoader(filename=os.fspath(path), sampleRate=sample_rate)()
    bpm, ticks, confidence, _estimates, _intervals = es.RhythmExtractor2013(
        method=method,
        minTempo=min_tempo,
        maxTempo=max_tempo,
    )(samples)
    return float(bpm), finite_ticks(ticks), float(confidence)


def main() -> int:
    args = parse_args()
    run_started = time.monotonic()
    if args.split == "held_out" and not args.allow_held_out:
        raise SystemExit("refusing held-out evaluation: pass --allow-held-out after freezing a tuning-selected configuration")
    if args.out.exists():
        raise SystemExit(f"refusing to overwrite existing output: {args.out}")
    if not args.manifest.is_file():
        raise SystemExit(f"manifest does not exist: {args.manifest}")
    if args.sample_rate <= 0 or args.min_tempo <= 0 or args.max_tempo <= args.min_tempo:
        raise SystemExit("sample rate and tempo range must be positive, with max tempo greater than min tempo")

    try:
        import essentia
    except ImportError as error:
        raise SystemExit("Essentia is required; create the isolated environment described in docs/REFERENCE_BPM_BENCHMARK.md") from error

    manifest = json.loads(args.manifest.read_text(encoding="utf-8"))
    path_base = args.path_base.resolve()
    result_rows: list[dict[str, Any]] = []
    for track in manifest.get("tracks", []):
        if track.get("split") != args.split:
            continue
        row: dict[str, Any] = {"id": track["id"]}
        try:
            bpm, ticks, confidence = analyze(
                resolve_path(track["path"], path_base), args.method, args.sample_rate, args.min_tempo, args.max_tempo
            )
            if bpm <= 0 or not math.isfinite(bpm) or not ticks:
                row["status"] = "unknown"
            else:
                row.update(status="partial", bpm=bpm, beatPositions=ticks)
                # RhythmExtractor2013 documents multifeature confidence on a
                # 0..5.32 scale. ResultSet confidence must remain 0..1.
                if args.method == "multifeature" and math.isfinite(confidence):
                    row["tempoConfidence"] = max(0.0, min(1.0, confidence / MULTIFEATURE_CONFIDENCE_MAX))
        except FileNotFoundError as error:
            row.update(status="failed", error="reference_source_unavailable", errorMessage=str(error))
        except RuntimeError as error:
            row.update(status="failed", error="reference_analysis_failed", errorMessage=str(error))
        except Exception as error:  # Preserve every reference-tool failure as evidence.
            row.update(status="failed", error="reference_analysis_failed", errorMessage=f"{type(error).__name__}: {error}")
        result_rows.append(row)

    if not result_rows:
        raise SystemExit(f"manifest has no {args.split!r} tracks")
    result_set = {
        "algorithm": f"essentia-{essentia.__version__}-rhythm-extractor-2013-{args.method}",
        "configuration": {
            "tool": "essentia.standard.RhythmExtractor2013",
            "toolVersion": essentia.__version__,
            "method": args.method,
            "sampleRate": args.sample_rate,
            "minTempo": args.min_tempo,
            "maxTempo": args.max_tempo,
            "beatPositionUnits": "seconds",
            "tempoConfidence": "multifeature raw confidence / 5.32, clamped to [0,1]" if args.method == "multifeature" else "unavailable for degara",
            "split": args.split,
            "manifest": args.manifest.name,
            "manifestSHA256": file_hash(args.manifest),
            "pathBase": os.fspath(path_base),
            "audioDurationPolicy": "full track",
            "wallSeconds": round(time.monotonic() - run_started, 6),
            "license": "AGPL-3.0 reference process only",
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
