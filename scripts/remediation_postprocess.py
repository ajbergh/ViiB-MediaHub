#!/usr/bin/env python3
"""Repair generated string literals before formatting and validation."""
from pathlib import Path

root = Path(__file__).resolve().parents[1]

api_path = root / "backend/internal/api/api.go"
api = api_path.read_text(encoding="utf-8")
api = api.replace(
    'strings.NewReplacer("/", "_", "\\", "_", ":", "_")',
    'strings.NewReplacer("/", "_", "\\\\", "_", ":", "_")',
)
api_path.write_text(api, encoding="utf-8", newline="\n")

spotify_path = root / "backend/internal/api/spotify.go"
spotify = spotify_path.read_text(encoding="utf-8")
spotify = spotify.replace(
    'fmt.Fprint(w, ": keepalive\n\n")',
    'fmt.Fprint(w, ": keepalive\\n\\n")',
)
spotify = spotify.replace(
    'fmt.Fprintf(w, "data: %s\n\n", data)',
    'fmt.Fprintf(w, "data: %s\\n\\n", data)',
)
spotify_path.write_text(spotify, encoding="utf-8", newline="\n")
