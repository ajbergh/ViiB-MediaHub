#!/usr/bin/env python3
"""Apply idempotent compile/static-analysis corrections to the reliability branch."""
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def update(path: str, transform) -> None:
    target = ROOT / path
    before = target.read_text(encoding="utf-8")
    after = transform(before)
    if after != before:
        target.write_text(after, encoding="utf-8")


def fix_core_types(text: str) -> str:
    old = "export type PlaybackContext = 'ai_dj' | 'album' | 'playlist' | 'queue' | 'search' | 'spotify';"
    new = "export type PlaybackContext = 'ai_dj' | 'album' | 'playlist' | 'queue' | 'search' | 'spotify' | 'artist' | 'liked' | 'smart_mix';"
    return text.replace(old, new)


def fix_slice_types(text: str) -> str:
    old_import = "import { Song, Playlist, SmartMix, ArtistMetadata, AlbumMetadata, SpotifyProfile, LogEntry, AudioSettings, VisualizerMode, ContextMenuType, MilkdropSettings, HomeLayoutVariant } from '../types';"
    new_import = "import { Song, Playlist, SmartMix, ArtistMetadata, AlbumMetadata, SpotifyProfile, LogEntry, AudioSettings, VisualizerMode, ContextMenuType, MilkdropSettings, HomeLayoutVariant, PlaybackContext } from '../types';"
    text = text.replace(old_import, new_import)
    text = text.replace("playbackContext?: import('../types').PlaybackContext", "playbackContext?: PlaybackContext")
    local_definition = """/**
 * Playback context for AI DJ preference learning.
 * Tracks where the user initiated playback from.
 */
export type PlaybackContext = 'ai_dj' | 'album' | 'playlist' | 'queue' | 'search' | 'artist' | 'liked' | 'smart_mix';

"""
    text = text.replace(local_definition, "")
    return text


def fix_download_manager(text: str) -> str:
    start = text.find("// refreshAccessToken refreshes an expired OAuth access token")
    end_marker = "// setAuthRequired updates the authRequired flag"
    end = text.find(end_marker)
    if start != -1 and end != -1 and end > start:
        text = text[:start] + text[end:]
    for package, token in (("io", "io."), ("net/http", "http."), ("strings", "strings.")):
        if token not in text:
            text = text.replace(f'\t"{package}"\n', "")
    return text


update("types.ts", fix_core_types)
update("slices/types.ts", fix_slice_types)
update("backend/internal/api/download_manager.go", fix_download_manager)

error_log = ROOT / "docs/REMEDIATION_RUN_ERROR.txt"
if error_log.exists():
    error_log.unlink()

print("Validation corrections applied")
