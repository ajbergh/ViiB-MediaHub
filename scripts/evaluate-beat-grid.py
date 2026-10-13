"""Compare exported beat ticks with a public reference grid at a fixed 70 ms.

No phase shift, tempo rescaling, or octave acceptance is optimized. Downbeats
and meter are not scored because the current benchmark does not export them.
"""
import argparse
import importlib.util
import json
import math
from pathlib import Path

spec = importlib.util.spec_from_file_location("structure_matching", Path(__file__).with_name("evaluate-structure-boundaries.py"))
matching = importlib.util.module_from_spec(spec)
spec.loader.exec_module(matching)


def evaluate(reference, results, split):
    truth = {}
    for row in reference["tracks"]:
        if row["id"] in truth:
            raise ValueError("Duplicate reference identity")
        truth[row["id"]] = row
    outputs = {}
    for row in results["results"]:
        if row["id"] in outputs or row["id"] not in truth or truth[row["id"]]["split"] != split:
            raise ValueError("Duplicate, unknown, or wrong-split result identity")
        outputs[row["id"]] = row
    rows = []
    for track_id, annotation in sorted(truth.items()):
        if annotation["split"] != split:
            continue
        duration = annotation["duration"]
        if not math.isfinite(duration) or duration <= 0:
            raise ValueError("Invalid reference duration")
        beats = annotation["beats"]
        matching.validate_boundaries(beats)
        if not beats or any(t >= duration + 1e-6 for t in beats):
            raise ValueError("Reference beats outside annotated span")
        output = outputs.get(track_id, {})
        valid = output.get("status") not in {"failed", "unsupported"} and not output.get("error") and not output.get("errorMessage")
        raw = output.get("beatPositions", []) if valid else []
        matching.validate_boundaries(raw)
        predicted = [t for t in raw if 0 <= t < duration]
        matched = matching.match_count(beats, predicted, 0.07)
        rows.append({"id": track_id, "referenceBeats": len(beats), "predictedBeats": len(predicted),
                     "excludedAfterAnnotatedSpan": len(raw) - len(predicted), "matched": matched,
                     "available": bool(predicted), **matching.rates(matched, len(beats), len(predicted))})
    if not rows:
        raise ValueError("No reference tracks in requested split")
    total_reference = sum(r["referenceBeats"] for r in rows)
    total_predicted = sum(r["predictedBeats"] for r in rows)
    total_matched = sum(r["matched"] for r in rows)
    return {"dataset": reference["dataset"], "algorithm": results["algorithm"], "configuration": results.get("configuration"),
            "split": split, "toleranceSeconds": 0.07,
            "method": "Maximum one-to-one beat matches in original seconds; no fitted offset, rescaling, or metrical alternatives; predictions clipped to annotated span; missing outputs count as misses.",
            "limitations": ["Reference is a release backing-track-derived grid, not manually labeled performed onsets.",
                            "Current benchmark exports beat ticks, not predicted downbeats or meter; those dimensions are unscored.",
                            "A fixed-window diagnostic is not continuity, perceptual, or broad genre qualification."],
            "missingBeatTracks": sum(not r["available"] for r in rows), "referenceBeats": total_reference,
            "predictedBeats": total_predicted, "matched": total_matched,
            "micro": matching.rates(total_matched, total_reference, total_predicted),
            "macroF1": sum(r["f1"] for r in rows) / len(rows), "tracks": rows}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("reference", type=Path)
    parser.add_argument("results", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--split", choices=["tuning", "held_out"], required=True)
    args = parser.parse_args()
    report = evaluate(json.loads(args.reference.read_text(encoding="utf-8")), json.loads(args.results.read_text(encoding="utf-8")), args.split)
    with args.output.open("x", encoding="utf-8") as target:
        json.dump(report, target, indent=2)
        target.write("\n")
