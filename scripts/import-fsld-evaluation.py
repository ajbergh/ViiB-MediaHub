"""Acquire only preselected CC0/CC-BY original FSLD audio via bounded ZIP ranges.

Resumes verified completed files. Keeps original bytes and attribution outside
the live library. A selection is not qualified by acquiring it.
"""
import argparse
from collections import Counter, defaultdict
import hashlib
import json
from pathlib import Path
import re
import struct
import time
import zipfile

from public_zip_ranges import PublicZipRanges

ARCHIVE_URL = "https://zenodo.org/records/3967852/files/FSL10K.zip?download=1"
ARCHIVE_SIZE = 8841802341
ARCHIVE_MD5 = "c260a1203a82c1dda80ec1de75e5e23f"
LICENSES = {"http://creativecommons.org/licenses/by/3.0/", "http://creativecommons.org/publicdomain/zero/1.0/"}


def wav_format(path):
    with path.open("rb") as source:
        header = source.read(12)
        if header[:4] != b"RIFF" or header[8:] != b"WAVE":
            return {"supported": False, "reason": "not-riff-wave"}
        while True:
            chunk = source.read(8)
            if len(chunk) != 8:
                return {"supported": False, "reason": "missing-fmt"}
            kind, length = chunk[:4], struct.unpack("<I", chunk[4:])[0]
            if kind == b"fmt ":
                if length < 16:
                    return {"supported": False, "reason": "short-fmt"}
                raw = source.read(16)
                if len(raw) != 16:
                    return {"supported": False, "reason": "truncated-fmt"}
                encoding, channels, rate, _, _, bits = struct.unpack("<HHIIHH", raw)
                supported = channels > 0 and rate > 0 and ((encoding == 1 and bits == 16) or (encoding == 3 and bits == 32))
                return {"supported": supported, "encoding": encoding, "channels": channels, "sampleRate": rate, "bits": bits}
            source.seek(length + length % 2, 1)


def sha256(path):
    with path.open("rb") as source:
        return hashlib.file_digest(source, "sha256").hexdigest()


def member_identity(name):
    # The pinned release uses audio/wav/<sound>_<uploader>.<source-ext>.wav,
    # including .wav.wav. Bind the sound ID exactly, never by substring.
    match = re.fullmatch(r"audio/wav/([0-9]+)_([0-9]+)\.[^/]+\.wav", name)
    return match[1] if match else None


def checkpoint(path, state):
    pending = path.with_suffix(".pending.json")
    pending.write_text(json.dumps(state, indent=2) + "\n", encoding="utf-8")
    for attempt in range(6):
        try:
            pending.replace(path)
            return
        except PermissionError as error:
            if getattr(error, "winerror", None) not in (5, 32, 33) or attempt == 5:
                raise
            time.sleep(.05 * 2 ** attempt)


def build_manifest(selection, recordings, audio_paths):
    # Fence identical bytes across splits; retain one copy per hash in each
    # split. Exclude all conflicting-split copies rather than moving labels.
    by_hash = defaultdict(list)
    for row in selection["recordings"]:
        acquired = recordings[row["id"]]
        if acquired.get("available") and acquired["format"]["supported"]:
            by_hash[acquired["sha256"]].append(row)
    tracks, exclusions = [], Counter()
    for digest, rows in sorted(by_hash.items()):
        if len({r["split"] for r in rows}) > 1:
            exclusions["identical-audio-across-splits"] += len(rows)
            continue
        labels = {(row["expectedBpm"], tuple(row["agreedKeyMode"]) if row.get("agreedKeyMode") else None) for row in rows}
        if len(labels) > 1:
            exclusions["duplicate-audio-conflicting-labels"] += len(rows)
            continue
        exclusions["duplicate-audio-within-split"] += len(rows) - 1
        row = sorted(rows, key=lambda r: r["id"])[0]
        key_pair = row.get("agreedKeyMode")
        key = ""
        if key_pair and len(key_pair) == 2 and isinstance(key_pair[0], str) and re.fullmatch(r"[a-g][#b]?", key_pair[0]) and key_pair[1] in ("maj", "min"):
            key = key_pair[0][0].upper() + key_pair[0][1:] + (" major" if key_pair[1] == "maj" else " minor")
        track = {"id": row["id"], "path": str(audio_paths[row["id"]].resolve()),
                 "license": row["license"] + "; attribution adjacent to original audio", "labelSource": selection["dataset"] + "; unanimous expert annotations",
                 "recordingGroup": row["recordingGroup"], "genre": "freesound-loop", "split": row["split"],
                 "expectedBpm": row["expectedBpm"], "notes": "Original broad genre labels retained in reserved selection; not automatically equated to required strata."}
        if key:
            track["expectedKey"] = key
        elif key_pair == ["none", "none"]:
            track["expectedKeyUnknown"] = True
        tracks.append(track)
    for value in recordings.values():
        if not value.get("available"):
            exclusions[value["reason"]] += 1
        elif not value["format"]["supported"]:
            exclusions["unsupported-production-wav-format"] += 1
    manifest = {"version": "fsld-unanimous-cc0-ccby-uploader-split-v1", "evidenceClass": "lawful-real-audio", "tracks": sorted(tracks, key=lambda r: r["id"])}
    return manifest, dict(exclusions)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("selection", type=Path)
    parser.add_argument("metadata", type=Path)
    parser.add_argument("output", type=Path, help="isolated resumable directory")
    args = parser.parse_args()
    selection = json.loads(args.selection.read_text(encoding="utf-8"))
    if sha256(args.metadata) != selection["metadataSHA256"]:
        raise ValueError("Metadata differs from reserved selection")
    metadata = json.loads(args.metadata.read_text(encoding="utf-8"))
    selection_hash = sha256(args.selection)
    args.output.mkdir(parents=True, exist_ok=True)
    (args.output / "audio").mkdir(exist_ok=True)
    state_path = args.output / "acquisition.json"
    state = json.loads(state_path.read_text(encoding="utf-8")) if state_path.exists() else {
        "selectionSHA256": selection_hash, "archiveURL": ARCHIVE_URL, "archiveBytes": ARCHIVE_SIZE,
        "officialArchiveMD5": ARCHIVE_MD5, "fullArchiveChecksumVerified": False, "recordings": {}}
    if state["selectionSHA256"] != selection_hash:
        raise ValueError("Output directory belongs to a different reserved selection")
    (args.output / "selection.json").write_bytes(args.selection.read_bytes())
    source = PublicZipRanges(ARCHIVE_URL, ARCHIVE_SIZE)
    with zipfile.ZipFile(source) as package:
        members = defaultdict(list)
        for info in package.infolist():
            identity = member_identity(info.filename)
            if identity:
                members[identity].append(info)
        for index, row in enumerate(selection["recordings"], 1):
            if not re.fullmatch(r"fsld-[0-9]+", row["id"]) or row["license"] not in LICENSES:
                raise ValueError("Invalid selected identity/license")
            sound_id = row["id"].removeprefix("fsld-")
            record = metadata[sound_id]
            if record["license"] != row["license"]:
                raise ValueError("Per-sound license differs from selection")
            target = args.output / "audio" / (row["id"] + ".wav")
            prior = state["recordings"].get(row["id"])
            candidates = members[sound_id]
            if len(candidates) != 1:
                state["recordings"][row["id"]] = {"available": False, "reason": "missing-or-ambiguous-archive-member"}
            elif prior and prior.get("available") and target.exists() and sha256(target) == prior["sha256"]:
                pass
            else:
                info = candidates[0]
                if info.file_size > 64 * 1024 * 1024 or info.compress_size + 131102 > source.max_range:
                    state["recordings"][row["id"]] = {"available": False, "reason": "source-member-exceeds-bound"}
                else:
                    source.prefetch(info.header_offset, info.compress_size + 131102)
                    partial = target.with_suffix(".part")
                    with package.open(info) as reader, partial.open("wb") as dest:
                        copied = 0
                        while data := reader.read(64 * 1024):
                            copied += len(data)
                            if copied > info.file_size:
                                raise ValueError("Member exceeds declared source size")
                            dest.write(data)
                    if copied != info.file_size:
                        raise ValueError("Truncated source member")
                    partial.replace(target)
                    attribution = {"creator": record["username"], "title": record["name"], "source": row["sourceURL"], "license": row["license"]}
                    target.with_suffix(".attribution.json").write_text(json.dumps(attribution, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
                    state["recordings"][row["id"]] = {"available": True, "sha256": sha256(target), "bytes": target.stat().st_size,
                        "zipMemberCRC32": f"{info.CRC:08x}", "zipMemberCRCVerified": True, "format": wav_format(target)}
            checkpoint(state_path, state)
            if index % 10 == 0 or index == len(selection["recordings"]):
                print(json.dumps({"processed": index, "total": len(selection["recordings"]), "sessionTransferredBytes": source.transferred,
                                  "available": sum(bool(r.get("available")) for r in state["recordings"].values())}), flush=True)
    audio_paths = {row["id"]: args.output / "audio" / (row["id"] + ".wav") for row in selection["recordings"]}
    manifest, exclusions = build_manifest(selection, state["recordings"], audio_paths)
    (args.output / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"manifestTracks": len(manifest["tracks"]), "splits": dict(Counter(t["split"] for t in manifest["tracks"])), "exclusions": dict(exclusions)}), flush=True)


if __name__ == "__main__":
    main()
