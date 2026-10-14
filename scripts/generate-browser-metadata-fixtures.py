"""Generate original one-second sine-wave fixtures with tags and embedded artwork.

Requires ffmpeg on PATH. Run from any directory; no downloaded audio is used.
"""
import base64
from pathlib import Path
import struct
import subprocess
import tempfile

destination = Path(__file__).resolve().parents[1] / 'testdata' / 'browser-metadata'
destination.mkdir(parents=True, exist_ok=True)
png = base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=')
# FLAC picture block, also used by Vorbis METADATA_BLOCK_PICTURE.
mime = b'image/png'
picture = struct.pack('>II', 3, len(mime)) + mime + struct.pack('>I', 0)
picture += struct.pack('>IIIII', 1, 1, 24, 0, len(png)) + png
with tempfile.TemporaryDirectory() as temporary:
    cover = Path(temporary) / 'cover.png'
    cover.write_bytes(png)
    for extension, codec in [('mp3', 'libmp3lame'), ('ogg', 'libvorbis'), ('flac', 'flac'), ('m4a', 'aac')]:
        command = ['ffmpeg', '-hide_banner', '-loglevel', 'error', '-y', '-f', 'lavfi', '-i', 'sine=frequency=440:duration=1']
        if extension != 'ogg':
            command += ['-i', str(cover), '-map', '0:a', '-map', '1:v', '-c:v', 'copy', '-disposition:v', 'attached_pic']
        else:
            command += ['-metadata', 'METADATA_BLOCK_PICTURE=' + base64.b64encode(picture).decode()]
        command += ['-c:a', codec]
        for key, value in [('title', 'Fixture Title'), ('artist', 'Fixture Artist'), ('album', 'Fixture Album'), ('album_artist', 'Fixture Album Artist'), ('track', '3'), ('disc', '1'), ('date', '2024'), ('genre', 'Electronic')]:
            command += ['-metadata', f'{key}={value}']
        command += [str(destination / f'tagged.{extension}')]
        subprocess.run(command, check=True)
