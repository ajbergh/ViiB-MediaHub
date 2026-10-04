/** Advance from server cursors, independent of fallback items or unequal buckets. */
export function nextSpotifySearchOffset(results: Record<string, any>): number | undefined {
    const offsets: number[] = [];
    for (const page of Object.values(results)) {
        if (!page?.next) continue;
        try {
            const value = new URL(page.next, 'https://api.spotify.com').searchParams.get('offset');
            const offset = value === null ? NaN : Number(value);
            if (Number.isInteger(offset) && offset >= 0) { offsets.push(offset); continue; }
        } catch { /* Use validated page metadata when a provider uses opaque links. */ }
        if (Number.isInteger(page.offset) && Number.isInteger(page.limit) && page.offset >= 0 && page.limit > 0) {
            offsets.push(page.offset + page.limit);
        }
    }
    return offsets.length ? Math.min(...offsets) : undefined;
}
