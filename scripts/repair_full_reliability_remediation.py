#!/usr/bin/env python3
from pathlib import Path

path = Path(__file__).with_name("apply_full_reliability_remediation.py")
text = path.read_text(encoding="utf-8")

old = '''replace_once(
    "backend/internal/api/spotify.go",
    'req.Header.Set("Authorization", "Bearer "+creds.AccessToken)',
    'req.Header.Set("Authorization", "Bearer "+accessToken)',
)
'''
if text.count(old) != 2:
    raise RuntimeError(f"expected two generic Spotify header replacements, found {text.count(old)}")

profile = '''regex_once(
    "backend/internal/api/spotify.go",
    r'(func \\(a \\*API\\) spotifyGetUserProfile.*?req\\.Header\\.Set\\("Authorization", "Bearer "\\+)creds\\.AccessToken(\\))',
    r'\\1accessToken\\2',
    flags=re.S,
)
'''
proxy = '''regex_once(
    "backend/internal/api/spotify.go",
    r'(func \\(a \\*API\\) spotifyProxy.*?req\\.Header\\.Set\\("Authorization", "Bearer "\\+)creds\\.AccessToken(\\))',
    r'\\1accessToken\\2',
    flags=re.S,
)
'''

text = text.replace(old, profile, 1)
text = text.replace(old, proxy, 1)
path.write_text(text, encoding="utf-8")
