"""Bounded, seekable HTTPS byte ranges for a pinned-size public ZIP archive."""
import io
import http.client
import time
import urllib.error
import urllib.request


class RangeTruncatedError(ValueError):
    pass


class PublicZipRanges(io.RawIOBase):
    def __init__(self, url, size, max_range=32 * 1024 * 1024):
        if not url.startswith("https://") or size <= 0 or max_range <= 0:
            raise ValueError("Expected HTTPS archive and positive bounds")
        self.url, self.size, self.max_range = url, size, max_range
        self.pos, self.transferred, self.requests = 0, 0, 0
        self.cache_start, self.cache = 0, b""

    def seekable(self):
        return True

    def readable(self):
        return True

    def tell(self):
        return self.pos

    def seek(self, offset, whence=0):
        if whence not in (0, 1, 2):
            raise ValueError("Invalid seek origin")
        position = offset + (self.pos if whence == 1 else self.size if whence == 2 else 0)
        if position < 0:
            raise ValueError("Negative seek")
        self.pos = position
        return position

    def prefetch(self, start, count):
        if start < 0 or count <= 0 or count > self.max_range or start >= self.size:
            raise ValueError("Invalid or oversized byte range")
        count = min(count, self.size - start)
        end = start + count - 1
        request = urllib.request.Request(self.url, headers={"Range": f"bytes={start}-{end}"})
        for attempt in range(4):
            try:
                with urllib.request.urlopen(request, timeout=60) as response:
                    if response.status != 206 or response.headers.get("Content-Range") != f"bytes {start}-{end}/{self.size}":
                        raise ValueError("Server did not honor exact byte range; body not downloaded")
                    data = response.read(count + 1)
                if len(data) < count:
                    raise RangeTruncatedError("Byte range response ended before the requested length")
                if len(data) > count:
                    raise ValueError("Byte range response exceeds requested length")
                break
            except urllib.error.HTTPError as error:
                if error.code != 429 or attempt == 3:
                    raise
                retry = error.headers.get("Retry-After", "60")
                delay = max(1, int(retry)) if retry.isdigit() else 60
                if delay > 3600:
                    raise
                print(f"Public archive rate limited; retrying in {delay} seconds", flush=True)
                while delay > 0:
                    interval = min(delay, 60)
                    time.sleep(interval)
                    delay -= interval
            except (urllib.error.URLError, TimeoutError, http.client.IncompleteRead, RangeTruncatedError):
                if attempt == 3:
                    raise
                print("Public archive transfer incomplete; retrying the exact bounded range", flush=True)
                time.sleep(2 ** attempt)
        self.cache_start, self.cache = start, data
        self.transferred += count
        self.requests += 1

    def read(self, count=-1):
        count = min(self.size - self.pos, count if count >= 0 else self.size - self.pos)
        if count <= 0:
            return b""
        if count > self.max_range:
            raise ValueError("Read exceeds range bound")
        if self.pos < self.cache_start or self.pos + count > self.cache_start + len(self.cache):
            self.prefetch(self.pos, count)
        offset = self.pos - self.cache_start
        data = self.cache[offset:offset + count]
        self.pos += count
        return data
