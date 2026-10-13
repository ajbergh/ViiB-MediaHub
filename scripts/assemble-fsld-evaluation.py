"""Verify acquisition batches against the original reserved selection and dedupe globally."""
import argparse
from collections import Counter
import importlib.util
import json
from pathlib import Path

spec = importlib.util.spec_from_file_location("fsld_import", Path(__file__).with_name("import-fsld-evaluation.py"))
corpus = importlib.util.module_from_spec(spec)
spec.loader.exec_module(corpus)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("selection", type=Path)
    parser.add_argument("output", type=Path, help="new assembled evidence directory; audio remains in batches")
    parser.add_argument("batches", type=Path, nargs="+")
    args = parser.parse_args()
    selection = json.loads(args.selection.read_text(encoding="utf-8"))
    original = {row["id"]: row for row in selection["recordings"]}
    if len(original) != len(selection["recordings"]):
        raise ValueError("Duplicate original selection identity")
    parent_hash = corpus.sha256(args.selection)
    rows, paths, batch_hashes = {}, {}, []
    for batch in args.batches:
        batch_plan_path = batch / "selection.json"
        batch_plan = json.loads(batch_plan_path.read_text(encoding="utf-8"))
        state = json.loads((batch / "acquisition.json").read_text(encoding="utf-8"))
        if batch_plan.get("parentSelectionSHA256", parent_hash) != parent_hash or batch_plan["metadataSHA256"] != selection["metadataSHA256"] or state["selectionSHA256"] != corpus.sha256(batch_plan_path):
            raise ValueError("Acquisition batch does not match reserved selection")
        batch_ids = {row["id"] for row in batch_plan["recordings"]}
        if len(batch_ids) != len(batch_plan["recordings"]) or set(state["recordings"]) != batch_ids:
            raise ValueError("Incomplete or ambiguous acquisition batch")
        for row in batch_plan["recordings"]:
            identity = row["id"]
            if identity in rows or original.get(identity) != row:
                raise ValueError("Batch identity/label/split differs from original selection")
            acquired = state["recordings"][identity]
            path = batch / "audio" / (identity + ".wav")
            if acquired.get("available"):
                if not acquired.get("zipMemberCRCVerified") or path.stat().st_size != acquired["bytes"] or corpus.sha256(path) != acquired["sha256"]:
                    raise ValueError("Acquired audio integrity mismatch")
                attribution = json.loads(path.with_suffix(".attribution.json").read_text(encoding="utf-8"))
                if attribution["source"] != row["sourceURL"] or attribution["license"] != row["license"] or not attribution.get("creator"):
                    raise ValueError("Missing or inconsistent per-sound attribution")
                acquired = {**acquired, "format": corpus.wav_format(path)}
            rows[identity], paths[identity] = acquired, path
        batch_hashes.append({"selectionSHA256": state["selectionSHA256"], "stateSHA256": corpus.sha256(batch / "acquisition.json")})
    if set(rows) != set(original):
        raise ValueError("Batches do not cover the entire original selection")
    manifest, exclusions = corpus.build_manifest(selection, rows, paths)
    admitted = {row["id"] for row in manifest["tracks"]}
    report = {"dataset": selection["dataset"], "selectionSHA256": parent_hash, "acquisitionBatches": batch_hashes,
              "selected": len(original), "available": sum(bool(r.get("available")) for r in rows.values()),
              "admitted": len(admitted), "splitCounts": dict(Counter(row["split"] for row in manifest["tracks"])),
              "expectedKeyRefusals": sum(bool(row.get("expectedKeyUnknown")) for row in manifest["tracks"]),
              "exclusions": exclusions, "recordings": [{"id": identity, "admitted": identity in admitted, **acquired} for identity, acquired in sorted(rows.items())],
              "limitations": ["Exact source-byte duplicates are fenced globally; transformed/near duplicates are not proven absent.",
                              "ZIP member CRC and per-file SHA256 verified; full remote archive MD5 not verified.",
                              "Format admission is preliminary; production decode results remain authoritative.",
                              "No model accuracy or broad acceptance follows from acquisition."]}
    args.output.mkdir(parents=True, exist_ok=False)
    (args.output / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    (args.output / "acquisition-report.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({key: report[key] for key in ("selected", "available", "admitted", "splitCounts", "expectedKeyRefusals", "exclusions")}))


if __name__ == "__main__":
    main()
