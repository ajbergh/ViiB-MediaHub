"""Reserve license-filtered, unanimous public FSLD labels before audio analysis.

This selects candidates, not an audio corpus. Per-sound audio presence, format,
duplicates, duration and attribution still require verification on acquisition.
"""
import argparse
from collections import Counter, defaultdict
import hashlib
import json
import math
from pathlib import Path
import zipfile

ANNOTATIONS_MD5 = "3920ee437802cf047a990b2968fa066c"
LICENSES = {"http://creativecommons.org/licenses/by/3.0/",
            "http://creativecommons.org/publicdomain/zero/1.0/"}


def select(votes, metadata):
    selected, reasons = [], Counter()
    for sound_id, rows in sorted(votes.items(), key=lambda item: int(item[0])):
        if len(rows) < 2:
            reasons["fewerThanTwoAnnotations"] += 1
            continue
        if any(row.get("discard") or row.get("save_for_later") for row in rows):
            reasons["discardedOrDeferredByAnyRater"] += 1
            continue
        tempos = []
        for row in rows:
            try:
                tempos.append(float(row["bpm"]) if row.get("defined_tempo") else None)
            except (TypeError, ValueError, KeyError):
                tempos.append(None)
        if any(t is None or not math.isfinite(t) or t <= 0 for t in tempos) or len(set(tempos)) != 1:
            reasons["tempoMissingOrDisagreement"] += 1
            continue
        record = metadata.get(sound_id)
        if not record or str(record.get("id")) != sound_id or not record.get("username"):
            reasons["metadataMissingOrInconsistent"] += 1
            continue
        if record.get("license") not in LICENSES:
            reasons["licenseOutsideCC0OrCCBY"] += 1
            continue
        if not all(row.get("well_cut") for row in rows):
            reasons["notUnanimouslyWellCut"] += 1
            continue
        group = hashlib.sha256(("fsld-uploader-v1:" + record["username"]).encode()).hexdigest()
        keys = {(row.get("key"), row.get("mode")) for row in rows}
        meters = {row.get("signature") for row in rows}
        selected.append({"id": "fsld-" + sound_id, "sourceURL": "https://freesound.org/s/" + sound_id + "/",
                         "license": record["license"], "recordingGroup": "fsld-uploader-" + group,
                         "split": "held_out" if int(group[:8], 16) % 3 == 0 else "tuning",
                         "expectedBpm": tempos[0], "reviewerCount": len(rows),
                         "agreedGenres": sorted(set.intersection(*(set(row.get("genres", [])) for row in rows))),
                         "agreedKeyMode": next(iter(keys)) if len(keys) == 1 else None,
                         "agreedMeter": next(iter(meters)) if len(meters) == 1 else None})
    return selected, dict(reasons)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("annotations", type=Path)
    parser.add_argument("metadata", type=Path)
    parser.add_argument("output", type=Path, help="new selection report; no audio paths or identities from the application")
    args = parser.parse_args()
    with args.annotations.open("rb") as source:
        if hashlib.file_digest(source, "md5").hexdigest() != ANNOTATIONS_MD5:
            raise ValueError("Official annotation checksum mismatch")
    votes, seen = defaultdict(list), set()
    with zipfile.ZipFile(args.annotations) as package:
        for name in package.namelist():
            if not name.endswith(".json") or name.startswith("__MACOSX/"):
                continue
            path = Path(name)
            sound_id, annotator = path.stem.removeprefix("sound-"), path.parent.name
            if not sound_id.isdigit() or (sound_id, annotator) in seen:
                raise ValueError("Invalid or duplicate sound/rater annotation")
            seen.add((sound_id, annotator))
            votes[sound_id].append(json.loads(package.read(name)))
    metadata = json.loads(args.metadata.read_text(encoding="utf-8"))
    selected, reasons = select(votes, metadata)
    report = {"dataset": "https://doi.org/10.5281/zenodo.3967852",
              "citation": "A. Ramires et al., The Freesound Loop Dataset and Annotation Tool, ISMIR 2020.",
              "stage": "Label/license selection only; no FSLD audio analyzed.",
              "annotationArchiveMD5": ANNOTATIONS_MD5,
              "metadataSHA256": hashlib.sha256(args.metadata.read_bytes()).hexdigest(),
              "metadataAcquisition": "Bounded HTTPS byte ranges from official FSL10K.zip; ZIP member CRC validated. Full 8.8 GB archive MD5 not verified.",
              "selectionPolicy": "At least two annotations; no rater discards/defers; all raters report the identical positive defined BPM and well_cut; per-sound license restricted to CC0 1.0 or CC BY 3.0. No detector output used.",
              "splitPolicy": "Every uploader is one group; SHA256 of fixed prefix fsld-uploader-v1: and uploader modulo 3 reserves one-third of uploader groups. Split before any FSLD analysis. Exact audio duplicates must be removed/fenced across splits after acquisition.",
              "selected": len(selected), "splitCounts": dict(Counter(row["split"] for row in selected)),
              "groups": len({row["recordingGroup"] for row in selected}),
              "agreedGenreCounts": dict(Counter(genre for row in selected for genre in row["agreedGenres"])),
              "agreedMeterCounts": dict(Counter(str(row["agreedMeter"]) for row in selected)),
              "excludedByFirstReason": reasons, "recordings": selected,
              "limitations": ["Original broad labels preserved: house / techno does not prove both strata; bass music does not prove drum-and-bass.",
                              "Short loops do not establish mastered full-track accuracy or long-track runtime behavior.",
                              "No audio presence, decoder support, duplicate-audio isolation, source duration or performance evidence established yet.",
                              "None key labels need dimension-specific abstention scoring; they are not both-dimension unknown labels.",
                              "Original per-sound attribution and license notices must accompany any downloaded audio."]}
    with args.output.open("x", encoding="utf-8") as target:
        json.dump(report, target, indent=2, ensure_ascii=False)
        target.write("\n")
    print(json.dumps({key: report[key] for key in ("selected", "splitCounts", "groups", "agreedGenreCounts")}))


if __name__ == "__main__":
    main()
