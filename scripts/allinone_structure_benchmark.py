#!/usr/bin/env python3
"""Run All-In-One-Infer as an isolated rhythm/structure reference process."""

from __future__ import annotations

import argparse
import importlib.metadata
import json
import os
from pathlib import Path
import time
from typing import Any

from rhythm_benchmark_common import (
    apply_overlap_audit, evidence_class, file_hash, normalized_result, normalized_row,
    validate_canonical_wav, write_result,
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True, help="new result JSON path; refuses to overwrite")
    parser.add_argument("--split", choices=("tuning", "held_out"), default="tuning")
    parser.add_argument("--allow-held-out", action="store_true", help="acknowledgement after freezing tuning configuration")
    parser.add_argument("--path-base", type=Path, default=Path.cwd())
    parser.add_argument("--model", default="harmonix-fold0", help="All-In-One-Infer model name")
    parser.add_argument("--overlap-audit", type=Path, help="matching training_overlap_audit.py report; exact manifest hash is enforced")
    parser.add_argument("--allow-model-download", action="store_true",
                        help="acknowledge that upstream may fetch weights if the selected model is not cached")
    return parser.parse_args()


def field(value: Any, key: str, default: Any = None) -> Any:
    return value.get(key, default) if isinstance(value, dict) else getattr(value, key, default)


def main() -> int:
    args = parse_args()
    started = time.monotonic()
    if args.split == "held_out" and not args.allow_held_out:
        raise SystemExit("refusing held-out evaluation: pass --allow-held-out after freezing a tuning-selected configuration")
    if args.out.exists():
        raise SystemExit(f"refusing to overwrite existing output: {args.out}")
    if not args.manifest.is_file():
        raise SystemExit(f"manifest does not exist: {args.manifest}")
    if not args.allow_model_download:
        raise SystemExit("refusing possible model download; pass --allow-model-download after reviewing the upstream model cache/license")
    try:
        import allin1_infer
    except ImportError as error:
        raise SystemExit("all-in-one-infer is required; install it only in an isolated development environment (see docs/REFERENCE_RHYTHM_STRUCTURE_BENCHMARK.md)") from error

    manifest = json.loads(args.manifest.read_text(encoding="utf-8"))
    path_base = args.path_base.resolve()
    overlap_audit_sha256 = apply_overlap_audit(manifest, args.manifest, args.overlap_audit)
    audio_timing: dict[str, Any] = {
        "inputRequirement": "ViiB-decoded canonical WAV path and timing record supplied by manifest",
        "timestampOrigin": "ViiB canonical decoded source frame 0; adapter performs no decoding or offset",
        "timingRecords": {},
    }
    result_rows: list[dict[str, Any]] = []
    for track in manifest.get("tracks", []):
        if track.get("split") != args.split:
            continue
        try:
            canonical, timing = validate_canonical_wav(track, path_base)
            kwargs: dict[str, Any] = {"paths": os.fspath(canonical), "model": args.model}
            result = allin1_infer.analyze(**kwargs)
            if isinstance(result, (list, tuple)):
                if len(result) != 1:
                    raise RuntimeError(f"expected one All-In-One result, got {len(result)}")
                result = result[0]
            beats = field(result, "beats", []) or []
            downbeats = field(result, "downbeats", []) or []
            segments = []
            for segment in field(result, "segments", []) or []:
                start = float(field(segment, "start"))
                end = float(field(segment, "end"))
                label = str(field(segment, "label", "unknown"))
                segments.append({"startSeconds": start, "endSeconds": end,
                                 "nativeLabel": label, "label": label})
            bpm_value = field(result, "bpm")
            bpm = float(bpm_value) if bpm_value is not None else None
            row = normalized_row(
                track["id"], status="partial" if beats or downbeats or bpm else "unknown",
                bpm=bpm, beats=beats, downbeats=downbeats, structure=segments,
                evidence_class=evidence_class(track),
            )
            positions = field(result, "beat_positions")
            if positions is not None:
                row["rhythm"]["beatPositionsInBar"] = [int(position) for position in positions]
            audio_timing["timingRecords"][track["id"]] = timing
            result_rows.append(row)
        except ValueError as error:
            result_rows.append(normalized_row(track["id"], status="blocked", error="canonical_audio_unavailable",
                                              error_message=str(error), evidence_class=evidence_class(track)))
        except FileNotFoundError as error:
            result_rows.append(normalized_row(track["id"], status="failed", error="reference_source_unavailable",
                                              error_message=str(error), evidence_class=evidence_class(track)))
        except Exception as error:  # Preserve external-tool failures as raw evidence.
            result_rows.append(normalized_row(track["id"], status="failed", error="reference_analysis_failed",
                                              error_message=f"{type(error).__name__}: {error}",
                                              evidence_class=evidence_class(track)))
    if not result_rows:
        raise SystemExit(f"manifest has no {args.split!r} tracks")
    try:
        version = importlib.metadata.version("all-in-one-infer")
    except importlib.metadata.PackageNotFoundError:
        version = getattr(allin1_infer, "__version__", "unknown")
    result = normalized_result(
        algorithm=f"all-in-one-infer-{version}-{args.model}",
        configuration={
            "tool": "allin1_infer.analyze",
            "toolVersion": version,
            "model": args.model,
            "modelArtifactSHA256": None,
            "modelArtifactIdentity": "package model registry entry; the documented analyze API does not return the resolved checkpoint path/hash",
            "split": args.split,
            "manifest": args.manifest.name,
            "manifestSHA256": file_hash(args.manifest),
            "overlapAudit": args.overlap_audit.name if args.overlap_audit else None,
            "overlapAuditSHA256": overlap_audit_sha256,
            "pathBase": os.fspath(path_base),
            "audioDurationPolicy": "full track",
            "wallSeconds": round(time.monotonic() - started, 6),
            "license": "MIT reference process only; review transitive model/runtime licenses separately",
            "purpose": "development-only external reference; not shipped or used by ViiB production analysis",
        },
        timing=audio_timing,
        results=result_rows,
    )
    write_result(args.out, result)
    print(json.dumps({"out": os.fspath(args.out), "tracks": len(result_rows), "split": args.split}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
