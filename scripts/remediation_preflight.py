#!/usr/bin/env python3
"""Adjust migration anchors that must target a specific reviewed code path."""
from pathlib import Path

path = Path(__file__).with_name("apply_full_remediation.py")
content = path.read_text(encoding="utf-8")

old = '''replace_once(
    "backend/internal/api/download_manager.go",
    '\\t\\tatomic.AddInt32(&dm.activeCount, -1)\\n',
    '',
)
'''
new = '''replace_once(
    "backend/internal/api/download_manager.go",
    '\\t\\tdelete(dm.downloadProgress, id)\\n\\t\\tatomic.AddInt32(&dm.activeCount, -1)\\n',
    '\\t\\tdelete(dm.downloadProgress, id)\\n',
)
'''

if old in content:
    content = content.replace(old, new, 1)

path.write_text(content, encoding="utf-8", newline="\n")
