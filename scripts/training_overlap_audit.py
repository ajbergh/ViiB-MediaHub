#!/usr/bin/env python3
"""Create a reproducible exact-metadata training-overlap report.

Input annotations must be a flattened JSON array/object or CSV containing
artist and title fields. The report calls records "clear" only when the caller
explicitly attests that the supplied annotation set is a complete coverage of
the stated model training data.
"""

from __future__ import annotations

import argparse
import csv
import json
from pathlib import Path
import re
import unicodedata

from rhythm_benchmark_common import file_hash


def normalize(value: object) -> str:
    text = unicodedata.normalize("NFKD", str(value or "")).casefold()
    text = "".join(char for char in text if not unicodedata.combining(char))
    return " ".join(re.sub(r"[^a-z0-9]+", " ", text).split())


def records(path: Path) -> list[dict[str, object]]:
    if path.suffix.casefold() == ".csv":
        with path.open("r", encoding="utf-8-sig", newline="") as source:
            return [dict(row) for row in csv.DictReader(source)]
    data = json.loads(path.read_text(encoding="utf-8"))
    if isinstance(data, list):
        return [item for item in data if isinstance(item, dict)]
    if isinstance(data, dict):
        for key in ("tracks", "annotations", "data"):
            if isinstance(data.get(key), list):
                return [item for item in data[key] if isinstance(item, dict)]
    raise ValueError(f"unsupported annotation structure: {path}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--annotations", type=Path, action="append", required=True,
                        help="flattened training-set JSON or CSV; may be repeated")
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--model", required=True, help="model/checkpoint and version being audited")
    parser.add_argument("--training-corpus", required=True, help="named training corpus represented by annotations")
    parser.add_argument("--artist-field", default="artist")
    parser.add_argument("--title-field", default="title")
    parser.add_argument("--annotation-id-field", default="id")
    parser.add_argument("--coverage-complete", action="store_true",
                        help="attest annotations cover the full stated training corpus; required to emit clear results")
    args = parser.parse_args()
    if args.out.exists():
        raise SystemExit(f"refusing to overwrite existing output: {args.out}")
    if not args.manifest.is_file() or any(not path.is_file() for path in args.annotations):
        raise SystemExit("manifest and all annotation files must exist")
    manifest = json.loads(args.manifest.read_text(encoding="utf-8"))
    annotation_rows = []
    for source in args.annotations:
        for row in records(source):
            artist, title = normalize(row.get(args.artist_field)), normalize(row.get(args.title_field))
            if artist and title:
                annotation_rows.append({"artist": artist, "title": title,
                                        "annotationId": str(row.get(args.annotation_id_field, "")),
                                        "source": source.name})
    index: dict[tuple[str, str], list[dict[str, str]]] = {}
    for row in annotation_rows:
        index.setdefault((row["artist"], row["title"]), []).append(row)
    items = []
    for track in manifest.get("tracks", []):
        artist, title = normalize(track.get("artist")), normalize(track.get("title"))
        matches = index.get((artist, title), []) if artist and title else []
        if matches:
            classification = "training-overlap-contaminated"
        elif args.coverage_complete and artist and title:
            classification = "overlap-reviewed-clear"
        else:
            classification = "overlap-unreviewed"
        items.append({
            "id": track.get("id"),
            "split": track.get("split"),
            "artist": track.get("artist"),
            "title": track.get("title"),
            "normalizedMatchKey": {"artist": artist, "title": title},
            "classification": classification,
            "trainingMatches": matches,
        })
    summary = {key: sum(item["classification"] == key for item in items) for key in (
        "training-overlap-contaminated", "overlap-reviewed-clear", "overlap-unreviewed")}
    result = {
        "schema": "viib.training-overlap-audit.v1",
        "model": args.model,
        "trainingCorpus": args.training_corpus,
        "matchingMethod": "exact normalized artist + title; no fuzzy matching",
        "coverageComplete": args.coverage_complete,
        "limitations": ["Exact metadata matches only; aliases, remasters, live versions and spelling variants may be missed."],
        "manifest": args.manifest.name,
        "manifestSHA256": file_hash(args.manifest),
        "annotationSources": [{"path": path.name, "sha256": file_hash(path)} for path in args.annotations],
        "summary": summary,
        "tracks": items,
    }
    args.out.parent.mkdir(parents=True, exist_ok=True)
    with args.out.open("x", encoding="utf-8", newline="\n") as destination:
        json.dump(result, destination, indent=2, ensure_ascii=False)
        destination.write("\n")
    print(json.dumps({"out": str(args.out), "tracks": len(items), "summary": summary}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
