#!/usr/bin/env python3
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def replace(path: str, old: str, new: str) -> None:
    target = ROOT / path
    text = target.read_text(encoding="utf-8")
    if old in text:
        target.write_text(text.replace(old, new), encoding="utf-8")


duplicate_fields = "  replayGainDb?: number;\n  replayPeak?: number;\n  replayGainDb?: number;\n  replayPeak?: number;\n"
replace("types.ts", duplicate_fields, "  replayGainDb?: number;\n  replayPeak?: number;\n")
replace("services/api.ts", duplicate_fields, "  replayGainDb?: number;\n  replayPeak?: number;\n")

duplicate_mapping = "    replayGainDb: apiSong.replayGainDb,\n    replayPeak: apiSong.replayPeak,\n    replayGainDb: apiSong.replayGainDb,\n    replayPeak: apiSong.replayPeak,\n"
replace("services/backendService.ts", duplicate_mapping, "    replayGainDb: apiSong.replayGainDb,\n    replayPeak: apiSong.replayPeak,\n")

replace(
    "backend/internal/db/db.go",
    "\t\tvar replayGainDB, replayPeak sql.NullFloat64\n\t\tvar replayGainDB, replayPeak sql.NullFloat64\n",
    "\t\tvar replayGainDB, replayPeak sql.NullFloat64\n",
)
replace(
    "backend/internal/db/db.go",
    "\t\t\treplay_gain_db = excluded.replay_gain_db,\n\t\t\treplay_peak = excluded.replay_peak,\n\t\t\treplay_gain_db = excluded.replay_gain_db,\n\t\t\treplay_peak = excluded.replay_peak,\n",
    "\t\t\treplay_gain_db = excluded.replay_gain_db,\n\t\t\treplay_peak = excluded.replay_peak,\n",
)

print("ReplayGain duplicate fields and SQL clauses removed")
