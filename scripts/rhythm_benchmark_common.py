"""Shared, development-only contract for beat/downbeat reference runs.

This module is intentionally not imported by MediaHub runtime code.
"""

from __future__ import annotations

import hashlib
import json
import math
import os
from pathlib import Path
from typing import Any, Iterable
import struct


SCHEMA = "viib.rhythm-reference.v1"


def file_hash(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1 << 20), b""):
            digest.update(block)
    return digest.hexdigest()


def resolve_path(value: str, path_base: Path) -> Path:
    candidate = Path(value.replace("\\", "/"))
    return candidate if candidate.is_absolute() else path_base / candidate


def finite_times(values: Iterable[Any], label: str) -> list[float]:
    times = [float(value) for value in values]
    if any(not math.isfinite(t) or t < 0 for t in times):
        raise RuntimeError(f"{label} contains a non-finite or negative timestamp")
    if any(right <= left for left, right in zip(times, times[1:])):
        raise RuntimeError(f"{label} timestamps are not strictly ascending")
    return times


def validate_canonical_wav(track: dict[str, Any], path_base: Path) -> tuple[Path, dict[str, Any]]:
    """Validate a WAV already decoded by ViiB; this function never decodes source audio."""
    canonical_path = track.get("canonicalWavPath")
    timing = track.get("canonicalAudio")
    if not canonical_path or not isinstance(timing, dict):
        raise ValueError("canonical ViiB WAV unavailable: manifest requires canonicalWavPath and canonicalAudio timing metadata")
    if timing.get("decoder") != "ViiB":
        raise ValueError("canonical WAV decoder must be recorded as ViiB")
    path = resolve_path(str(canonical_path), path_base)
    if not path.is_file():
        raise FileNotFoundError(f"canonical ViiB WAV not found: {path}")
    try:
        wav = path.open("rb")
    except OSError as error:
        raise FileNotFoundError(f"cannot open canonical ViiB WAV: {error}") from error
    with wav:
        header = wav.read(12)
        if len(header) != 12 or header[:4] != b"RIFF" or header[8:] != b"WAVE":
            raise ValueError("canonical ViiB audio must be a little-endian RIFF/WAVE file")
        fmt: tuple[int, int, int, int, int, int] | None = None
        data_bytes: int | None = None
        data_offset: int | None = None
        while True:
            chunk_header = wav.read(8)
            if not chunk_header:
                break
            if len(chunk_header) != 8:
                raise ValueError("truncated WAV chunk header")
            chunk_id, chunk_size = struct.unpack("<4sI", chunk_header)
            if chunk_id == b"fmt ":
                if chunk_size < 16 or chunk_size > 4096:
                    raise ValueError("WAV fmt chunk size is outside the supported range")
                payload = wav.read(chunk_size)
                if len(payload) < 16:
                    raise ValueError("WAV fmt chunk is too short")
                fmt = struct.unpack("<HHIIHH", payload[:16])
            elif chunk_id == b"data":
                data_bytes = chunk_size
                data_offset = wav.tell()
                wav.seek(chunk_size, os.SEEK_CUR)
            else:
                wav.seek(chunk_size, os.SEEK_CUR)
            if chunk_size & 1:
                wav.seek(1, os.SEEK_CUR)
        if fmt is None or data_bytes is None or data_offset is None:
            raise ValueError("canonical WAV is missing fmt or data chunk")
        format_tag, channels, sample_rate, byte_rate, block_align, bits = fmt
        encoding = "pcm_s16le" if format_tag == 1 and bits == 16 else "float32le" if format_tag == 3 and bits == 32 else None
        if encoding is None:
            raise ValueError("canonical ViiB WAV encoding must be PCM16 or IEEE float32")
        if channels not in (1, 2) or sample_rate <= 0 or block_align != channels * bits // 8 or byte_rate != sample_rate * block_align or data_bytes % block_align:
            raise ValueError("canonical WAV has invalid channel/rate/block geometry")
        frames = data_bytes // block_align
        if frames <= 0:
            raise ValueError("canonical WAV has no audio frames")
    expected = {"encoding": encoding, "channels": channels, "sampleRate": sample_rate, "frames": frames}
    for key, actual in expected.items():
        if timing.get(key) != actual:
            raise ValueError(f"canonicalAudio.{key} does not match WAV header (expected {actual!r})")
    if not timing.get("decoderVersion"):
        raise ValueError("canonicalAudio.decoderVersion is required")
    source_hash = timing.get("sourceAudioSHA256")
    if not isinstance(source_hash, str) or len(source_hash) != 64 or any(char not in "0123456789abcdef" for char in source_hash):
        raise ValueError("canonicalAudio.sourceAudioSHA256 must identify the decoded source record")
    if timing.get("timelineOriginSeconds") != 0:
        raise ValueError("canonical ViiB WAV timelineOriginSeconds must be 0")
    for field_name in ("resampleDelaySamples", "startTrimSamples"):
        if timing.get(field_name) != 0:
            raise ValueError(f"canonicalAudio.{field_name} must be 0; this adapter does not remap trimmed or delayed audio")
    pcm_digest = hashlib.sha256()
    remaining = data_bytes
    with path.open("rb") as source:
        source.seek(data_offset)
        while True:
            block = source.read(min(1 << 20, remaining))
            if not block:
                break
            pcm_digest.update(block)
            remaining -= len(block)
        if remaining:
            raise ValueError("canonical WAV data chunk is truncated")
    if pcm_digest.hexdigest() != source_hash:
        raise ValueError("canonicalAudio.sourceAudioSHA256 does not match WAV PCM data")
    wav_digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1 << 20), b""):
            wav_digest.update(block)
    return path, {
        "canonicalWavPath": os.fspath(path),
        "container": "WAV",
        **expected,
        "durationSeconds": frames / sample_rate,
        "wavFileSHA256": wav_digest.hexdigest(),
        "canonicalPCMSHA256": source_hash,
        "sourceAudioSHA256": source_hash,
        "decoder": "ViiB",
        "decoderVersion": timing["decoderVersion"],
        "resampleDelaySamples": timing["resampleDelaySamples"],
        "startTrimSamples": timing["startTrimSamples"],
        "timestampOrigin": "ViiB canonical decoded source frame 0",
        "timeMapping": "reference timestamps in seconds are returned on ViiB canonical timeline; adapter applies no offset",
    }


def normalized_result(*, algorithm: str, configuration: dict[str, Any], timing: dict[str, Any],
                      results: list[dict[str, Any]]) -> dict[str, Any]:
    return {
        "schema": SCHEMA,
        "algorithm": algorithm,
        "configuration": configuration,
        "audioTiming": timing,
        "results": results,
    }


def normalized_row(track_id: str, *, status: str, bpm: float | None = None,
                   beats: Iterable[Any] = (), downbeats: Iterable[Any] = (),
                   beat_confidences: Iterable[Any] = (), downbeat_confidences: Iterable[Any] = (),
                   meter: dict[str, Any] | None = None, structure: list[dict[str, Any]] | None = None,
                   error: str | None = None, error_message: str | None = None,
                   evidence_class: str = "unreviewed",
                   downbeat_provenance: str | None = None) -> dict[str, Any]:
    beat_times = finite_times(beats, "beats")
    downbeat_times = finite_times(downbeats, "downbeats")
    allowed_provenance = {"measured", "manual", "inferred-from-meter", "unknown"}
    provenance = downbeat_provenance or ("measured" if downbeat_times else "unknown")
    if provenance not in allowed_provenance:
        raise ValueError(f"unsupported downbeat provenance: {provenance}")
    row: dict[str, Any] = {
        "id": track_id,
        "status": status,
        "evidenceClass": evidence_class,
        "rhythm": {
            "tempoBpm": bpm if bpm is not None and math.isfinite(float(bpm)) and float(bpm) > 0 else None,
            "beats": [{"timeSeconds": time} for time in beat_times],
            "downbeats": [{"timeSeconds": time, "provenance": provenance,
                           "provenanceScope": "benchmark-reference-prediction" if provenance == "measured" else "annotation-or-derived-grid"}
                          for time in downbeat_times],
            "meter": meter,
            "structure": structure or [],
            "downbeatProvenance": provenance,
            "downbeatProvenanceScope": "benchmark-reference-prediction" if provenance == "measured" else "annotation-or-derived-grid",
        },
    }
    beat_conf = list(beat_confidences)
    downbeat_conf = list(downbeat_confidences)
    for key, values, target in (("beats", beat_conf, beat_times), ("downbeats", downbeat_conf, downbeat_times)):
        if values:
            if len(values) != len(target):
                raise RuntimeError(f"{key} confidence count does not match timestamp count")
            if any(not math.isfinite(float(value)) or not 0 <= float(value) <= 1 for value in values):
                raise RuntimeError(f"{key} confidence must be finite and within [0,1]")
            for item, confidence in zip(row["rhythm"][key], values):
                item["confidence"] = float(confidence)
    # Legacy fields keep existing BPM comparison tooling usable.
    if row["rhythm"]["tempoBpm"] is not None:
        row["bpm"] = row["rhythm"]["tempoBpm"]
    row["beatPositions"] = beat_times
    row["downbeatPositions"] = downbeat_times
    if error:
        row["error"] = error
    if error_message:
        row["errorMessage"] = error_message
    return row


def write_result(path: Path, result: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("x", encoding="utf-8", newline="\n") as destination:
        json.dump(result, destination, indent=2, allow_nan=False)
        destination.write("\n")


def evidence_class(track: dict[str, Any]) -> str:
    """Keep overlap contamination independent from tuning/held-out assignment."""
    overlap = track.get("trainingOverlap", track.get("overlapStatus", "unreviewed"))
    if isinstance(overlap, dict):
        overlap = overlap.get("status", "unreviewed")
    if overlap in (True, "overlap", "contaminated", "matched"):
        return "training-overlap-contaminated"
    if overlap in (False, "clear", "no-match"):
        return "overlap-reviewed-clear"
    return "overlap-unreviewed"


def apply_overlap_audit(manifest: dict[str, Any], manifest_path: Path, audit_path: Path | None) -> str | None:
    if audit_path is None:
        return None
    audit = json.loads(audit_path.read_text(encoding="utf-8"))
    if audit.get("schema") != "viib.training-overlap-audit.v1":
        raise ValueError("unsupported training-overlap audit schema")
    if audit.get("manifestSHA256") != file_hash(manifest_path):
        raise ValueError("overlap audit was generated for a different manifest revision")
    classifications = {str(row.get("id")): row.get("classification") for row in audit.get("tracks", [])}
    if set(classifications) != {str(row.get("id")) for row in manifest.get("tracks", [])}:
        raise ValueError("overlap audit track IDs do not exactly match the manifest")
    for track in manifest["tracks"]:
        classification = classifications[str(track["id"])]
        track["trainingOverlap"] = {
            "training-overlap-contaminated": "contaminated",
            "overlap-reviewed-clear": "clear",
            "overlap-unreviewed": "unreviewed",
        }.get(classification, "unreviewed")
    return file_hash(audit_path)


def persist_downbeat_provenance(*, measured: bool = False, manual: bool = False,
                                meter_derived: bool = False) -> str:
    """Common vocabulary intended for later production serialization, not a detector."""
    choices = sum((measured, manual, meter_derived))
    if choices > 1:
        raise ValueError("downbeat provenance choices are mutually exclusive")
    if manual:
        return "manual"
    if measured:
        return "measured"
    if meter_derived:
        return "inferred-from-meter"
    return "unknown"
