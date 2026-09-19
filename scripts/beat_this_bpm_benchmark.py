#!/usr/bin/env python3
"""Emit benchmark-only Beat This BPM and beat ticks.

Run this only in an isolated development environment. Beat This and its model
remain external reference evidence and are never imported, linked, or shipped
by ViiB.
"""

from __future__ import annotations

import argparse
import hashlib
import importlib.metadata
import json
import math
import os
from pathlib import Path
import statistics
import time
from typing import Any


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True, help="new result JSON path; refuses to overwrite")
    parser.add_argument("--split", choices=("tuning", "held_out"), default="tuning")
    parser.add_argument("--allow-held-out", action="store_true", help="required acknowledgement before evaluating held-out audio")
    parser.add_argument("--path-base", type=Path, default=Path.cwd(), help="base for relative manifest paths (default: current directory)")
    parser.add_argument("--checkpoint", type=Path, required=True, help="local final0 checkpoint; adapter never downloads model weights")
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


def finite_ticks(values: Any) -> list[float]:
    ticks = [float(value) for value in values]
    if any(not math.isfinite(tick) or tick < 0 for tick in ticks):
        raise RuntimeError("Beat This returned an invalid beat tick")
    if any(right <= left for left, right in zip(ticks, ticks[1:])):
        raise RuntimeError("Beat This returned non-ascending beat ticks")
    return ticks


def bpm_from_ticks(ticks: list[float]) -> float | None:
    intervals = [right - left for left, right in zip(ticks, ticks[1:]) if right > left and math.isfinite(right - left)]
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
    # Disable the optional madmom DBN to keep this reference path self-contained
    # and retain Beat This's published minimal postprocessor behavior.
    detector = File2Beats(checkpoint_path=os.fspath(args.checkpoint), device="cpu", dbn=False)
    rows: list[dict[str, Any]] = []
    for track in manifest.get("tracks", []):
        if track.get("split") != args.split:
            continue
        row: dict[str, Any] = {"id": track["id"]}
        try:
            beats, _downbeats = detector(os.fspath(resolve_path(track["path"], path_base)))
            ticks = finite_ticks(beats)
            bpm = bpm_from_ticks(ticks)
            if bpm is None:
                row["status"] = "unknown"
            else:
                row.update(status="partial", bpm=bpm, beatPositions=ticks)
        except FileNotFoundError as error:
            row.update(status="failed", error="reference_source_unavailable", errorMessage=str(error))
        except RuntimeError as error:
            row.update(status="failed", error="reference_analysis_failed", errorMessage=str(error))
        except Exception as error:  # Preserve every external-tool failure as evidence.
            row.update(status="failed", error="reference_analysis_failed", errorMessage=f"{type(error).__name__}: {error}")
        rows.append(row)
    if not rows:
        raise SystemExit(f"manifest has no {args.split!r} tracks")
    result_set = {
        "algorithm": f"beat-this-{importlib.metadata.version('beat-this')}-final0-minimal",
        "configuration": {
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
            "bpmDerivation": "60 / median(positive consecutive beat-position intervals)",
            "torchVersion": torch.__version__,
            "torchaudioVersion": torchaudio.__version__,
            "split": args.split,
            "manifest": args.manifest.name,
            "manifestSHA256": file_hash(args.manifest),
            "pathBase": os.fspath(path_base),
            "audioDurationPolicy": "full track",
            "wallSeconds": round(time.monotonic() - started, 6),
            "license": "MIT reference process only; model weights remain external development artifacts",
            "purpose": "development-only external reference; not shipped or used by ViiB production analysis",
        },
        "results": rows,
    }
    with args.out.open("x", encoding="utf-8", newline="\n") as destination:
        json.dump(result_set, destination, indent=2)
        destination.write("\n")
    print(json.dumps({"out": os.fspath(args.out), "tracks": len(rows), "split": args.split}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
