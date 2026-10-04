/** Combines saved-library pages using server positions and stable totals while detecting incomplete traversal. */

interface LibraryPage<T> {
    items: T[];
    offset: number;
    total: number;
    next?: string | null;
}

// Count provider positions, including unavailable entries and repeated resources.
export function appendSpotifyLibraryPage<T>(previous: LibraryPage<T>, page: LibraryPage<T>): LibraryPage<T> {
    const offset = previous.offset + previous.items.length;
    if (!Array.isArray(previous.items) || !Array.isArray(page.items) ||
        !Number.isInteger(previous.offset) || previous.offset !== 0 ||
        !Number.isInteger(page.total) || page.total < 0 || page.total !== previous.total ||
        page.offset !== offset || page.items.length === 0 ||
        offset + page.items.length > page.total ||
        Boolean(page.next) !== (offset + page.items.length < page.total)) {
        throw new Error('Spotify library changed or returned an invalid page');
    }
    return { ...page, offset: previous.offset, items: [...previous.items, ...page.items] };
}
