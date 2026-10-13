"""Score interior section boundaries, separately from beat/downbeat annotations.

This evaluates energy-region boundaries against musical-form boundaries; it
does not assert equivalent semantics or score the predicted section labels.
"""
import argparse
import json
import math
from pathlib import Path


def validate_boundaries(values):
    if any(not isinstance(v, (int, float)) or isinstance(v, bool) or not math.isfinite(v) or v < 0 for v in values):
        raise ValueError("Boundary times must be finite nonnegative seconds")
    if any(b <= a for a, b in zip(values, values[1:])):
        raise ValueError("Boundary times must be strictly ascending")


def match_count(reference, predicted, tolerance):
    """Maximum one-to-one match count for sorted times and a fixed window."""
    validate_boundaries(reference)
    validate_boundaries(predicted)
    if not math.isfinite(tolerance) or tolerance < 0:
        raise ValueError("Invalid matching tolerance")
    i = j = matched = 0
    while i < len(reference) and j < len(predicted):
        if abs(reference[i] - predicted[j]) <= tolerance:
            matched += 1
            i += 1
            j += 1
        elif predicted[j] < reference[i]:
            j += 1
        else:
            i += 1
    return matched


def rates(matched, reference, predicted):
    precision = matched / predicted if predicted else 0.0
    recall = matched / reference if reference else 0.0
    return {"precision": precision, "recall": recall,
            "f1": 2 * matched / (reference + predicted) if reference + predicted else None}


def evaluate(reference, results):
    result_map = {}
    for row in results["results"]:
        if row["id"] in result_map:
            raise ValueError("Duplicate detector identity")
        result_map[row["id"]] = row
    if set(result_map) - set(reference["tracks"]):
        raise ValueError("Detector output contains identities outside this reference corpus")
    reports = []
    for tolerance in [0.5, 3.0]:
        tracks = []
        for identity, annotation in sorted(reference["tracks"].items()):
            start, end = annotation["annotatedStart"], annotation["annotatedEnd"]
            if not math.isfinite(start) or not math.isfinite(end) or start < 0 or end <= start:
                raise ValueError("Invalid annotated span")
            truth = annotation["boundaries"]
            validate_boundaries(truth)
            if any(t <= start or t >= end for t in truth):
                raise ValueError("Reference boundary outside its annotated interior")
            output = result_map.get(identity, {})
            available = output.get("structureAvailable", False) and output.get("status") not in {"failed", "unsupported"} and not output.get("error") and not output.get("errorMessage")
            raw = output.get("structureBoundaries", []) if available else []
            validate_boundaries(raw)
            duration = output.get("audioDurationSeconds", 0)
            if available and (not math.isfinite(duration) or duration <= 0 or end > duration + 0.1 or any(t <= 0 or t >= duration for t in raw)):
                raise ValueError("Detector/reference duration or interior boundary mismatch")
            predicted = [t for t in raw if start < t < end]
            matched = match_count(truth, predicted, tolerance)
            tracks.append({"id": identity, "available": bool(available), "referenceBoundaries": len(truth),
                           "predictedBoundaries": len(predicted), "excludedOutsideAnnotatedInterior": len(raw) - len(predicted),
                           "matched": matched, **rates(matched, len(truth), len(predicted))})
        total_reference = sum(t["referenceBoundaries"] for t in tracks)
        total_predicted = sum(t["predictedBoundaries"] for t in tracks)
        total_matched = sum(t["matched"] for t in tracks)
        defined = [t["f1"] for t in tracks if t["f1"] is not None]
        reports.append({"toleranceSeconds": tolerance, "tracks": tracks,
                        "missingStructureTracks": sum(not t["available"] for t in tracks),
                        "referenceBoundaries": total_reference, "predictedBoundaries": total_predicted,
                        "matched": total_matched, "micro": rates(total_matched, total_reference, total_predicted),
                        "macroF1": sum(defined) / len(defined) if defined else None})
    return {"method": "Maximum one-to-one temporal matches; interior boundaries only; predictions clipped to annotated interior; unavailable outputs count as misses.",
            "dataset": reference["dataset"], "algorithm": results["algorithm"], "configuration": results.get("configuration"),
            "limitations": ["Energy-derived regions and annotated musical-form regions have different semantics; labels are not scored.",
                            "No BPM/beat/downbeat accuracy or broad qualification follows from this boundary comparison."],
            "windows": reports}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("reference", type=Path)
    parser.add_argument("results", type=Path)
    parser.add_argument("output", type=Path, help="new report path")
    args = parser.parse_args()
    report = evaluate(json.loads(args.reference.read_text(encoding="utf-8")), json.loads(args.results.read_text(encoding="utf-8")))
    with args.output.open("x", encoding="utf-8") as target:
        json.dump(report, target, indent=2)
        target.write("\n")
