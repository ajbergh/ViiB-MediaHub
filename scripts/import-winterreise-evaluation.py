"""Import the official SWD 2.1 HU33 recordings into an isolated held-out corpus.

Audio stays outside tracked files. No Spotify values, synthetic tempo labels,
or tuning decisions are used. Run from the repository root.
"""
import argparse
import csv
import hashlib
import io
import json
import math
from pathlib import Path
import re
import shutil
import zipfile

DOI = "https://doi.org/10.5281/zenodo.10839767"
ARCHIVE_MD5 = "591c377c6d3db522fd159b8b70180978"


def csv_rows(data):
    return list(csv.DictReader(io.StringIO(data.decode("utf-8-sig")), delimiter=";"))


def global_keys(data):
    labels = {}
    for row in csv_rows(data):
        if row["PerformanceID"] != "HU33":
            continue
        work, value = row["WorkID"], row["key"]
        match = re.fullmatch(r"([A-G][#b]?):(maj|min)", value)
        if not match or work in labels:
            raise ValueError("Invalid or duplicate HU33 global-key annotation")
        labels[work] = match[1] + (" major" if match[2] == "maj" else " minor")
    expected = {f"Schubert_D911-{n:02d}" for n in range(1, 25)}
    if set(labels) != expected:
        raise ValueError("Expected exactly the 24 HU33 key annotations")
    return labels


def structure_reference(data):
    rows = csv_rows(data)
    sections = []
    for row in rows:
        start, end = float(row["start"]), float(row["end"])
        if not math.isfinite(start) or not math.isfinite(end) or start < 0 or end <= start:
            raise ValueError("Invalid structure interval")
        if sections and start < sections[-1]["end"] - 1e-6:
            raise ValueError("Overlapping or unordered structure intervals")
        sections.append({"start": start, "end": end, "label": row["structure"]})
    if not sections:
        raise ValueError("Missing structure intervals")
    return {"annotatedStart": sections[0]["start"], "annotatedEnd": sections[-1]["end"],
            "boundaries": [row["start"] for row in sections[1:]], "sections": sections}


def import_corpus(archive, output):
    with archive.open("rb") as source:
        digest = hashlib.file_digest(source, "md5").hexdigest()
    if digest != ARCHIVE_MD5:
        raise ValueError("Archive differs from the official SWD 2.1 checksum")
    # Exact known archive members are copied; archive-supplied paths are never
    # used as output paths, preventing extraction into unrelated directories.
    with zipfile.ZipFile(archive) as package:
        labels = global_keys(package.read("02_Annotations/ann_audio_globalkey.csv"))
        references = {}
        for work in sorted(labels):
            identity = work + "_HU33"
            references[identity] = structure_reference(package.read(
                f"02_Annotations/ann_audio_structure/{identity}.csv"))
        license_bytes = package.read("03_ExtraMaterial/license_HU33.txt")
        if b"publicdomain/mark/1.0" not in license_bytes:
            raise ValueError("HU33 public-domain notice missing")
        output.mkdir(parents=True, exist_ok=False)
        (output / "audio").mkdir()
        tracks, hashes = [], {}
        for work, key in sorted(labels.items()):
            identity = work + "_HU33"
            path = output / "audio" / (identity + ".wav")
            with package.open(f"01_RawData/audio_wav/{identity}.wav") as source, path.open("xb") as target:
                shutil.copyfileobj(source, target)
            with path.open("rb") as source:
                hashes[identity] = hashlib.file_digest(source, "sha256").hexdigest()
            tracks.append({"id": identity, "path": str(path.resolve()),
                           "license": "HU33 audio: Public Domain Mark 1.0; dataset/annotations: CC BY 3.0",
                           "labelSource": DOI + " / 02_Annotations/ann_audio_globalkey.csv (HU33)",
                           "recordingGroup": identity, "genre": "classical-art-song",
                           "coverage": ["acoustic", "sparse-no-percussion"], "split": "held_out",
                           "expectedKey": key,
                           "notes": "Reserved evaluation only. No global BPM label. Song01 uses the release's annotation-matched artificial repeat."})
        manifest = {"version": "swd-2.1-hu33-held-out-v1", "evidenceClass": "lawful-real-audio", "tracks": tracks}
        reference = {"dataset": DOI, "archiveMD5": digest, "audioSHA256": hashes,
                     "annotationSource": "02_Annotations/ann_audio_structure/*_HU33.csv",
                     "boundaryDefinition": "Section starts after the first annotated section; annotated span endpoints excluded.",
                     "tracks": references}
        for name, value in [("manifest.json", manifest), ("structure-reference.json", reference)]:
            (output / name).write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")
        (output / "license_HU33.txt").write_bytes(license_bytes)
        (output / "README-upstream.txt").write_bytes(package.read("README.txt"))
    return manifest


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("output", type=Path, help="new isolated directory, preferably under .tmp-review")
    args = parser.parse_args()
    result = import_corpus(args.archive, args.output)
    print(f"Reserved {len(result['tracks'])} HU33 recordings; key labels only, no BPM ground truth.")
