#!/usr/bin/env python3
"""Emit benchmark-only Beat This BPM and beat ticks.

Run this only in an isolated development environment. Beat This and its model
remain external reference evidence and are never imported, linked, or shipped
by ViiB.
"""

from __future__ import annotations

import argparse
import importlib.metadata
import json
import math
import os
from pathlib import Path
import statistics
import time
from typing import Any

from rhythm_benchmark_common import (
    apply_overlap_audit, evidence_class, file_hash, normalized_result, normalized_row,
    write_result,
    validate_canonical_wav,
)


# Beat This emits ticks at 50 FPS. A one-beat interval alone is quantized in
# 20 ms steps (for example 140 BPM can appear as 142.86); sixteen beats spans
# four 4/4 bars and gives the BPM scalar usable resolution while the median
# still rejects isolated missed or spurious ticks.
BPM_PERIOD_SPAN = 16


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True, help="new result JSON path; refuses to overwrite")
    parser.add_argument("--split", choices=("tuning", "held_out"), default="tuning")
    parser.add_argument("--allow-held-out", action="store_true", help="required acknowledgement before evaluating held-out audio")
    parser.add_argument("--path-base", type=Path, default=Path.cwd(), help="base for relative manifest paths (default: current directory)")
    parser.add_argument("--checkpoint", type=Path, required=True, help="local final0 checkpoint; adapter never downloads model weights")
    parser.add_argument("--overlap-audit", type=Path, help="matching training_overlap_audit.py report; exact manifest hash is enforced")
    return parser.parse_args()


def finite_ticks(values: Any) -> list[float]:
    ticks = [float(value) for value in values]
    if any(not math.isfinite(tick) or tick < 0 for tick in ticks):
        raise RuntimeError("Beat This returned an invalid beat tick")
    if any(right <= left for left, right in zip(ticks, ticks[1:])):
        raise RuntimeError("Beat This returned non-ascending beat ticks")
    return ticks


def bpm_from_ticks(ticks: list[float]) -> float | None:
    intervals = [
        (ticks[index + BPM_PERIOD_SPAN] - ticks[index]) / BPM_PERIOD_SPAN
        for index in range(len(ticks) - BPM_PERIOD_SPAN)
        if ticks[index + BPM_PERIOD_SPAN] > ticks[index] and math.isfinite(ticks[index + BPM_PERIOD_SPAN] - ticks[index])
    ]
    if not intervals:
        return None
    bpm = 60.0 / statistics.median(intervals)
    return bpm if math.isfinite(bpm) and bpm > 0 else None


def main() -> int:
    args = parse_args()
    started = time.monotonic()
    if args.split == "held_out" and not args.allow_held_out:
        raise SystemExit("refusing held-out evaluation: pass --allow-held-out after freezing a tuning-selected configuration")
    if args.out.exists():
        raise SystemExit(f"refusing to overwrite existing output: {args.out}")
    if not args.manifest.is_file():
        raise SystemExit(f"manifest does not exist: {args.manifest}")
    if not args.checkpoint.is_file():
        raise SystemExit(f"checkpoint does not exist: {args.checkpoint}")
    try:
        import torch
        import torchaudio
        from beat_this.inference import File2Beats
    except ImportError as error:
        raise SystemExit("Beat This is required; create the isolated environment described in docs/REFERENCE_BPM_BENCHMARK.md") from error

    manifest = json.loads(args.manifest.read_text(encoding="utf-8"))
    path_base = args.path_base.resolve()
    overlap_audit_sha256 = apply_overlap_audit(manifest, args.manifest, args.overlap_audit)
    # Disable the optional madmom DBN to keep this reference path self-contained
    # and retain Beat This's published minimal postprocessor behavior.
    detector = File2Beats(checkpoint_path=os.fspath(args.checkpoint), device="cpu", dbn=False)
    audio_timing: dict[str, Any] = {
        "inputRequirement": "ViiB-decoded canonical WAV path and timing record supplied by manifest",
        "timestampOrigin": "ViiB canonical decoded source frame 0; adapter performs no decoding or offset",
        "timingRecords": {},
    }
    result_rows: list[dict[str, Any]] = []
    for track in manifest.get("tracks", []):
        if track.get("split") != args.split:
            continue
        row: dict[str, Any]
        try:
            canonical, timing = validate_canonical_wav(track, path_base)
            beats, downbeats = detector(os.fspath(canonical))
            beat_ticks = finite_ticks(beats)
            downbeat_ticks = finite_ticks(downbeats)
            bpm = bpm_from_ticks(beat_ticks)
            audio_timing["timingRecords"][track["id"]] = timing
            row = normalized_row(
                track["id"], status="partial" if bpm is not None or beat_ticks else "unknown",
                bpm=bpm, beats=beat_ticks, downbeats=downbeat_ticks,
                evidence_class=evidence_class(track),
            )
        except ValueError as error:
            row = normalized_row(track["id"], status="blocked", error="canonical_audio_unavailable",
                                 error_message=str(error), evidence_class=evidence_class(track))
        except FileNotFoundError as error:
            row = normalized_row(track["id"], status="failed", error="reference_source_unavailable",
                                 error_message=str(error), evidence_class=evidence_class(track))
        except RuntimeError as error:
            row = normalized_row(track["id"], status="failed", error="reference_analysis_failed",
                                 error_message=str(error), evidence_class=evidence_class(track))
        except Exception as error:  # Preserve every external-tool failure as evidence.
            row = normalized_row(track["id"], status="failed", error="reference_analysis_failed",
                                 error_message=f"{type(error).__name__}: {error}",
                                 evidence_class=evidence_class(track))
        result_rows.append(row)
    if not result_rows:
        raise SystemExit(f"manifest has no {args.split!r} tracks")
    result_set = normalized_result(
        algorithm=f"beat-this-{importlib.metadata.version('beat-this')}-final0-minimal",
        configuration={
            "tool": "beat_this.inference.File2Beats",
            "toolVersion": importlib.metadata.version("beat-this"),
            "model": "final0",
            "checkpointSHA256": file_hash(args.checkpoint),
            "checkpointPath": os.fspath(args.checkpoint),
            "device": "cpu",
            "dbn": False,
            "postprocessor": "minimal",
            "targetSampleRate": 22050,
            "beatPositionUnits": "seconds",
            "bpmDerivation": "60 / median((beatPosition[i + 16] - beatPosition[i]) / 16)",
            "bpmPeriodSpanBeats": BPM_PERIOD_SPAN,
            "torchVersion": torch.__version__,
            "torchaudioVersion": torchaudio.__version__,
            "split": args.split,
            "manifest": args.manifest.name,
            "manifestSHA256": file_hash(args.manifest),
            "overlapAudit": args.overlap_audit.name if args.overlap_audit else None,
            "overlapAuditSHA256": overlap_audit_sha256,
            "pathBase": os.fspath(path_base),
            "audioDurationPolicy": "full track",
            "wallSeconds": round(time.monotonic() - started, 6),
            "license": "MIT reference process only; model weights remain external development artifacts",
            "purpose": "development-only external reference; not shipped or used by ViiB production analysis",
        },
        timing=audio_timing,
        results=result_rows,
    )
    write_result(args.out, result_set)
    print(json.dumps({"out": os.fspath(args.out), "tracks": len(result_rows), "split": args.split}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
