/** Opt-in smoke check against a running, connected local MediaHub instance. */
import assert from 'node:assert/strict';

const origin = process.argv[2] ?? 'http://localhost:3000';
const target = new URL(origin);
assert(['localhost', '127.0.0.1', '[::1]'].includes(target.hostname), 'Use a local MediaHub server');

async function read(offset, limit) {
    const url = new URL('/api/spotify/proxy', target);
    url.search = new URLSearchParams({path: 'me/player/recently-played', offset, limit});
    const response = await fetch(url, {signal: AbortSignal.timeout(20000)});
    assert(response.ok, `Recents returned HTTP ${response.status} at offset ${offset}`
        + (response.headers.has('Retry-After') ? ` (Retry-After ${response.headers.get('Retry-After')})` : ''));
    const page = await response.json();
    assert.equal(page.source, 'webplayer_recents');
    assert.equal(page.offset, offset);
    assert(Array.isArray(page.items));
    return page;
}

const first = await read(0, 50);
assert(Number.isInteger(first.next_offset), 'History needs a second page to run this check');
const next = await read(first.next_offset, first.limit);
assert.equal(next.total, first.total, 'History changed during the smoke check');
const terminal = next.next_offset === null ? next : await read(first.total - 1, first.limit);
assert.equal(terminal.total, first.total, 'History changed during the smoke check');
assert.equal(terminal.next_offset, null);
assert.equal(terminal.offset + terminal.items.length, terminal.total);
console.log('Spotify Recents first, next, and terminal pages passed.');
