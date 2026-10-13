"""Import checksum-pinned GuitarSet mic audio and JAMS labels without transcoding.

Reserve every style's lead-sheet index 3 (both tempi, all players, comp/solo)
before analysis. Index 1/2 recordings are tuning data. Audio stays untracked.
"""
import argparse
from collections import Counter
import hashlib
import json
import math
from pathlib import Path
import re
import shutil
import zipfile

DOI = "https://doi.org/10.5281/zenodo.3371780"
ARCHIVES = {"annotation": "b39b78e63d3446f2e54ddb7a54df9b10",
            "audio": "275966d6610ac34999b58426beb119c3"}
EXCLUDED = {"04_BN3-154-E_comp": "upstream timing offset +0.409 seconds (issue 5)",
            "04_Jazz1-200-B_comp": "upstream timing offset +0.309 seconds (issue 5)",
            "02_Funk2-119-G_comp": "upstream duplicated MIDI note (issue 4)"}
STYLES = {"BN": "bossa-nova", "Funk": "funk", "Jazz": "jazz",
          "Rock": "rock", "SS": "singer-songwriter"}


def identity(stem):
    match = re.fullmatch(r"(0[0-5])_((BN|Funk|Jazz|Rock|SS)([1-3]))-(\d+)-([A-G][#b]?)_(comp|solo)", stem)
    if not match:
        raise ValueError("Unexpected GuitarSet recording identity: " + stem)
    return {"player": match[1], "group": match[2], "style": STYLES[match[3]],
            "split": "held_out" if match[4] == "3" else "tuning",
            "filenameTempo": int(match[5]), "filenameTonic": match[6], "part": match[7]}


def labels(document, stem):
    info = identity(stem)
    if document["file_metadata"]["title"] != stem:
        raise ValueError("JAMS title does not match recording identity")
    duration = float(document["file_metadata"]["duration"])
    if not math.isfinite(duration) or duration <= 0:
        raise ValueError("Invalid annotation duration")
    annotations = document["annotations"]
    def only(namespace):
        matches = [a["data"] for a in annotations if a["namespace"] == namespace]
        if len(matches) != 1 or not matches[0]:
            raise ValueError("Missing or duplicate " + namespace + " annotation")
        return matches[0]
    tempo, key, beats = only("tempo"), only("key_mode"), only("beat_position")
    if len(tempo) != 1 or len(key) != 1:
        raise ValueError("Expected one global tempo and key")
    bpm = float(tempo[0]["value"])
    key_match = re.fullmatch(r"([A-G][#b]?):(major|minor)", key[0]["value"])
    if not math.isfinite(bpm) or bpm <= 0 or bpm != info["filenameTempo"] or not key_match:
        raise ValueError("Invalid or inconsistent tempo/key label")
    if key_match[1] != info["filenameTonic"]:
        raise ValueError("Key annotation does not match filename tonic")
    times, downbeats, meter = [], [], set()
    for beat in beats:
        time, value = float(beat["time"]), beat["value"]
        if not math.isfinite(time) or time < 0 or time >= duration + 1e-6 or (times and time <= times[-1]):
            raise ValueError("Invalid or unordered beat annotation")
        position, count, units = value["position"], value["num_beats"], value["beat_units"]
        if not all(isinstance(v, int) and v > 0 for v in (position, count, units)) or position > count:
            raise ValueError("Invalid beat position/meter")
        meter.add((count, units))
        times.append(time)
        if position == 1:
            downbeats.append(time)
    # The release describes a backing-track/click-derived grid, not manually
    # measured performed onsets. Check that this is internally consistent.
    if any(abs((b - a) - 60 / bpm) > 1e-5 for a, b in zip(times, times[1:])):
        raise ValueError("Tempo and beat grid disagree")
    return info, bpm, key_match[1] + " " + key_match[2], {
        "duration": duration, "beats": times, "downbeats": downbeats,
        "meters": [list(pair) for pair in sorted(meter)],
        "labelSemantics": "release global metadata and backing-track-derived beat grid; not performed onset labels"}


def checked_zip(path, expected):
    with path.open("rb") as source:
        actual = hashlib.file_digest(source, "md5").hexdigest()
    if actual != expected:
        raise ValueError("Official archive checksum mismatch: " + path.name)
    return zipfile.ZipFile(path)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("annotations", type=Path)
    parser.add_argument("audio", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    with checked_zip(args.annotations, ARCHIVES["annotation"]) as annotations, checked_zip(args.audio, ARCHIVES["audio"]) as audio:
        jams = {Path(n).stem: n for n in annotations.namelist()
                if n.endswith(".jams") and not n.startswith("__MACOSX/")}
        wavs = {Path(n).stem.removesuffix("_mic"): n for n in audio.namelist()
                if n.endswith("_mic.wav") and not n.startswith("__MACOSX/")}
        if len(jams) != 360 or set(jams) != set(wavs) or not set(EXCLUDED) <= set(jams):
            raise ValueError("Expected exactly 360 matching JAMS/mic recordings and known exclusions")
        parsed = {stem: labels(json.loads(annotations.read(name)), stem) for stem, name in sorted(jams.items())}
        args.output.mkdir(parents=True, exist_ok=False)
        audio_dir = args.output / "audio"
        audio_dir.mkdir()
        tracks, references, hashes = [], [], []
        for stem, (info, bpm, key, grid) in parsed.items():
            if stem in EXCLUDED:
                continue
            target = audio_dir / (stem + "_mic.wav")
            with audio.open(wavs[stem]) as source, target.open("xb") as dest:
                shutil.copyfileobj(source, dest)
            with target.open("rb") as source:
                digest = hashlib.file_digest(source, "sha256").hexdigest()
            track_id = "guitarset-" + stem
            tracks.append({"id": track_id, "path": str(target.resolve()), "license": "CC BY 4.0; " + DOI,
                           "labelSource": DOI + " annotation.zip / " + Path(jams[stem]).name,
                           "recordingGroup": "guitarset-leadsheet-" + info["group"],
                           "genre": "acoustic-guitar-" + info["style"], "coverage": ["acoustic"],
                           "split": info["split"], "expectedBpm": bpm, "expectedKey": key,
                           "notes": "Global release labels; acoustic guitar excerpt, not live drums. Both tempi/all players/comp+solo share split."})
            references.append({"id": track_id, "split": info["split"], **grid})
            hashes.append({"id": track_id, "sha256": digest, "bytes": target.stat().st_size})
        write = lambda name, value: (args.output / name).write_text(json.dumps(value, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
        write("manifest.json", {"version": "guitarset-1.1.0-mic-split-v1", "evidenceClass": "lawful-real-audio", "tracks": tracks})
        write("grid-reference.json", {"dataset": DOI, "tracks": references})
        write("provenance.json", {"dataset": DOI, "license": "CC BY 4.0", "archiveMD5": ARCHIVES,
              "citation": "Q. Xi, R. Bittner, J. Pauwels, X. Ye, J. P. Bello, GuitarSet: A Dataset for Guitar Transcription, ISMIR 2018.",
              "splitPolicy": "Reserve lead-sheet index 3 across all styles/tempi/players/parts; index 1/2 tuning. Declared before analysis.",
              "splitCounts": dict(Counter(t["split"] for t in tracks)), "excluded": EXCLUDED, "audio": hashes,
              "limitations": ["Short solo acoustic guitar excerpts do not cover required electronic/live-drum strata.",
                               "Global tempo/key and backing-track-derived grids are independent of Spotify but not manually performed onset annotations.",
                               "Shared harmonic vocabulary may remain across lead-sheet groups; no composition-independence claim."]})
        print(json.dumps({"tracks": len(tracks), "splits": dict(Counter(t["split"] for t in tracks)), "excluded": len(EXCLUDED)}))


if __name__ == "__main__":
    main()
