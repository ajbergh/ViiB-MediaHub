/** Displays podcast chapters, scrolling lyrics-style transcripts, bookmarks, and show notes inside the Now Playing drawer and stage. */

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useInRouterContext, useNavigate } from 'react-router';
import {
    Bookmark,
    BookOpen,
    ChevronLeft,
    ChevronRight,
    Crosshair,
    ExternalLink,
    FastForward,
    FileText,
    ListOrdered,
    Loader2,
    LocateFixed,
    MessageSquareText,
    Minus,
    Pause,
    Play,
    Plus,
    RotateCcw,
    Search,
    Trash2,
    X,
} from 'lucide-react';
import { useStore } from '../../store';
import {
    podcastsAPI,
    type PodcastBookmark,
    type PodcastChapter,
    type PodcastEpisodeDetail,
    type PodcastTranscriptCue,
} from '../../services/podcasts';
import type { Song } from '../../types';
import { Button } from '../ui/Button';

type PodcastSubTab = 'chapters' | 'transcript' | 'bookmarks' | 'notes';

interface PodcastDrawerPanelProps {
    song: Song;
    currentTime: number;
    onSeek: (time: number) => void;
    onNavigate?: () => void;
    isVisible?: boolean;
}

interface EpisodeNavButtonProps {
    episodeId: string;
    onNavigate?: () => void;
    className?: string;
    title?: string;
    'aria-label'?: string;
    children: React.ReactNode;
}

const RouterEpisodeButton: React.FC<EpisodeNavButtonProps> = ({
    episodeId,
    onNavigate,
    children,
    ...rest
}) => {
    const navigate = useNavigate();
    const to = `/podcasts/episode/${encodeURIComponent(episodeId)}`;
    return (
        <button
            type="button"
            onClick={() => {
                navigate(to);
                onNavigate?.();
            }}
            {...rest}
        >
            {children}
        </button>
    );
};

export const EpisodeNavButton: React.FC<EpisodeNavButtonProps> = (props) => {
    const inRouter = useInRouterContext();
    if (inRouter) {
        return <RouterEpisodeButton {...props} />;
    }
    const to = `/podcasts/episode/${encodeURIComponent(props.episodeId)}`;
    return (
        <a
            href={to}
            onClick={(e) => {
                e.preventDefault();
                window.history.pushState({}, '', to);
                props.onNavigate?.();
            }}
            className={props.className}
            title={props.title}
            aria-label={props['aria-label']}
        >
            {props.children}
        </a>
    );
};

function formatTimestamp(seconds: number): string {
    if (!Number.isFinite(seconds) || seconds < 0) return '0:00';
    const total = Math.floor(seconds);
    const hrs = Math.floor(total / 3600);
    const mins = Math.floor((total % 3600) / 60);
    const secs = total % 60;
    if (hrs > 0) {
        return `${hrs}:${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
    }
    return `${mins}:${secs.toString().padStart(2, '0')}`;
}

function sortBookmarks(items: PodcastBookmark[]): PodcastBookmark[] {
    return [...items].sort((a, b) => a.positionMs - b.positionMs || a.createdAt - b.createdAt);
}

const BLOCKED_HTML_TAGS = new Set([
    'script',
    'style',
    'iframe',
    'object',
    'embed',
    'svg',
    'math',
    'template',
    'noscript',
    'form',
    'input',
    'textarea',
    'select',
    'button',
    'meta',
    'link',
    'base',
]);

function safeChapterImageURL(raw?: string): string | null {
    if (!raw) return null;
    const trimmed = raw.trim();
    if (!trimmed) return null;
    try {
        const parsed = new URL(trimmed);
        return parsed.protocol === 'https:' ? parsed.toString() : null;
    } catch {
        return null;
    }
}

function renderRichNode(node: ChildNode, key: string): React.ReactNode {
    if (node.nodeType === Node.TEXT_NODE) {
        return node.textContent;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) {
        return null;
    }
    const el = node as Element;
    const tag = el.tagName.toLowerCase();
    if (BLOCKED_HTML_TAGS.has(tag)) {
        return null;
    }
    const children = Array.from(el.childNodes).map((child, idx) => renderRichNode(child, `${key}-${idx}`));

    switch (tag) {
        case 'br':
            return <br key={key} />;
        case 'p':
            return (
                <p key={key} className="mb-2.5 last:mb-0 leading-relaxed text-text-secondary">
                    {children}
                </p>
            );
        case 'strong':
        case 'b':
            return (
                <strong key={key} className="font-semibold text-text-main">
                    {children}
                </strong>
            );
        case 'em':
        case 'i':
            return (
                <em key={key} className="italic">
                    {children}
                </em>
            );
        case 'ul':
            return (
                <ul key={key} className="list-disc pl-5 mb-2.5 space-y-1 text-text-secondary">
                    {children}
                </ul>
            );
        case 'ol':
            return (
                <ol key={key} className="list-decimal pl-5 mb-2.5 space-y-1 text-text-secondary">
                    {children}
                </ol>
            );
        case 'li':
            return <li key={key}>{children}</li>;
        case 'blockquote':
            return (
                <blockquote key={key} className="border-l-2 border-brand/50 pl-3 my-2.5 italic text-text-secondary">
                    {children}
                </blockquote>
            );
        case 'code':
            return (
                <code key={key} className="px-1.5 py-0.5 rounded bg-surface-3 font-mono text-[11px] text-text-main">
                    {children}
                </code>
            );
        case 'a': {
            const href = el.getAttribute('href') || '';
            if (href.startsWith('https://') || href.startsWith('http://')) {
                return (
                    <a
                        key={key}
                        href={href}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-brand underline hover:text-brand-hover break-words"
                    >
                        {children}
                    </a>
                );
            }
            return <React.Fragment key={key}>{children}</React.Fragment>;
        }
        default:
            return <React.Fragment key={key}>{children}</React.Fragment>;
    }
}

function renderSafeRichDescription(richHtml: string | undefined, plainText: string | undefined): React.ReactNode {
    const trimmedRich = richHtml?.trim();
    if (trimmedRich && typeof DOMParser !== 'undefined') {
        const doc = new DOMParser().parseFromString(trimmedRich, 'text/html');
        const nodes = Array.from(doc.body.childNodes).map((child, idx) => renderRichNode(child, `rich-${idx}`));
        if (nodes.length > 0) {
            return <div className="text-xs leading-relaxed text-text-secondary break-words">{nodes}</div>;
        }
    }
    const fallback = plainText?.trim();
    if (fallback) {
        return <p className="text-xs leading-relaxed text-text-secondary whitespace-pre-wrap break-words">{fallback}</p>;
    }
    return <p className="text-xs text-text-subtle">No episode notes available.</p>;
}

function readSavedTranscriptOffsetMs(episodeId?: string): number {
    if (!episodeId || typeof localStorage === 'undefined') return 0;
    try {
        const raw = localStorage.getItem(`viib:podcast-transcript-offset:${episodeId}`);
        if (!raw) return 0;
        const parsed = Number(raw);
        return Number.isFinite(parsed) ? Math.round(parsed) : 0;
    } catch {
        return 0;
    }
}

function saveTranscriptOffsetMs(episodeId: string | undefined, offsetMs: number): void {
    if (!episodeId) return;
    try {
        if (offsetMs === 0) {
            localStorage.removeItem(`viib:podcast-transcript-offset:${episodeId}`);
        } else {
            localStorage.setItem(`viib:podcast-transcript-offset:${episodeId}`, String(Math.round(offsetMs)));
        }
    } catch {
        // Ignore restricted browser storage environments.
    }
    window.dispatchEvent(
        new CustomEvent('viib:podcast-transcript-offset-changed', {
            detail: { episodeId, offsetMs: Math.round(offsetMs) },
        }),
    );
}

function formatOffsetLabel(offsetMs: number): string {
    const secs = Math.round(offsetMs / 1000);
    if (secs === 0) return '0s';
    return `${secs > 0 ? '+' : '−'}${Math.abs(secs)}s`;
}

interface PodcastTranscriptLyricsProps {
    episodeId?: string;
    cues: PodcastTranscriptCue[];
    chapters?: PodcastChapter[];
    currentTime: number;
    onSeek: (time: number) => void;
    isVisible: boolean;
    presentation?: 'drawer' | 'stage';
    query?: string;
    errorMessage?: string | null;
}

/**
 * Renders podcast transcript cues with ViiB's centered, blurred-inactive
 * scrolling-lyrics presentation, auto-centering tracking, and DAI sponsor-break
 * pause/sync controls.
 */
export const PodcastTranscriptLyrics: React.FC<PodcastTranscriptLyricsProps> = ({
    episodeId,
    cues,
    chapters = [],
    currentTime,
    onSeek,
    isVisible,
    presentation = 'drawer',
    query = '',
    errorMessage,
}) => {
    const onStage = presentation === 'stage';
    const viewportRef = useRef<HTMLDivElement>(null);
    const activeLineRef = useRef<HTMLDivElement>(null);
    const resumeTimerRef = useRef<number | null>(null);
    const [autoFollow, setAutoFollow] = useState(true);
    const [offsetMs, setOffsetMs] = useState<number>(() => readSavedTranscriptOffsetMs(episodeId));
    const [pausedAtSec, setPausedAtSec] = useState<number | null>(null);
    const [alignMode, setAlignMode] = useState(false);

    useEffect(() => {
        setOffsetMs(readSavedTranscriptOffsetMs(episodeId));
        setPausedAtSec(null);
        setAlignMode(false);
    }, [episodeId]);

    useEffect(() => {
        if (!episodeId) return;
        const onSyncChanged = (event: Event) => {
            const custom = event as CustomEvent<{ episodeId?: string; offsetMs?: number }>;
            if (custom.detail?.episodeId === episodeId && typeof custom.detail.offsetMs === 'number') {
                setOffsetMs(custom.detail.offsetMs);
            }
        };
        window.addEventListener('viib:podcast-transcript-offset-changed', onSyncChanged);
        return () => window.removeEventListener('viib:podcast-transcript-offset-changed', onSyncChanged);
    }, [episodeId]);

    const updateOffsetMs = (nextMs: number) => {
        const clamped = Math.max(-3600_000, Math.min(3600_000, Math.round(nextMs)));
        setOffsetMs(clamped);
        saveTranscriptOffsetMs(episodeId, clamped);
    };

    const nonContentChapter = useMemo(() => {
        return chapters.find(
            (ch) =>
                ch.toc === false &&
                currentTime >= ch.startTime &&
                (ch.endTime === undefined || currentTime < ch.endTime),
        );
    }, [chapters, currentTime]);

    const trimmedQuery = query.trim().toLowerCase();
    const effectivePlaybackSec = pausedAtSec !== null ? pausedAtSec : currentTime;
    const currentTimeMs = Math.max(0, Math.round(effectivePlaybackSec * 1000) - offsetMs);

    const activeCueIndex = useMemo(() => {
        return cues.reduce((best, cue, idx) => {
            if (currentTimeMs >= cue.startMs && (cue.endMs <= cue.startMs || currentTimeMs < cue.endMs)) {
                return idx;
            }
            return best;
        }, -1);
    }, [cues, currentTimeMs]);

    const activeCue = activeCueIndex >= 0 ? cues[activeCueIndex] : null;

    const filteredCues = useMemo(() => {
        if (!trimmedQuery) return cues;
        return cues.filter(
            (cue) =>
                cue.text.toLowerCase().includes(trimmedQuery) ||
                (cue.speaker ? cue.speaker.toLowerCase().includes(trimmedQuery) : false),
        );
    }, [cues, trimmedQuery]);

    useEffect(() => {
        return () => {
            if (resumeTimerRef.current !== null) {
                window.clearTimeout(resumeTimerRef.current);
            }
        };
    }, []);

    const pauseAutoFollowTemporarily = () => {
        if (trimmedQuery) return;
        setAutoFollow(false);
        if (resumeTimerRef.current !== null) {
            window.clearTimeout(resumeTimerRef.current);
        }
        resumeTimerRef.current = window.setTimeout(() => {
            resumeTimerRef.current = null;
            setAutoFollow(true);
        }, 6000);
    };

    const centerActiveLine = (instant = false) => {
        const viewport = viewportRef.current;
        const line = activeLineRef.current;
        if (!viewport || !line || typeof viewport.scrollTo !== 'function') return;
        const lineRect = line.getBoundingClientRect();
        const viewportRect = viewport.getBoundingClientRect();
        const relativeTop = lineRect.top - viewportRect.top;
        const reduced =
            typeof window.matchMedia === 'function' &&
            window.matchMedia('(prefers-reduced-motion: reduce)').matches;
        viewport.scrollTo({
            top: viewport.scrollTop + relativeTop - (viewport.clientHeight - lineRect.height) / 2,
            behavior: instant || reduced ? 'instant' : 'smooth',
        });
    };

    useEffect(() => {
        if (!isVisible || trimmedQuery || !autoFollow || pausedAtSec !== null || activeCueIndex < 0) return;
        centerActiveLine(false);
    }, [activeCueIndex, isVisible, presentation, trimmedQuery, autoFollow, pausedAtSec, cues.length]);

    const handleToggleBreakPause = () => {
        if (pausedAtSec === null) {
            setPausedAtSec(currentTime);
            setAlignMode(false);
            return;
        }
        const elapsedBreakMs = Math.max(0, Math.round((currentTime - pausedAtSec) * 1000));
        setPausedAtSec(null);
        updateOffsetMs(offsetMs + elapsedBreakMs);
        setAutoFollow(true);
    };

    const handleCueAction = (cue: PodcastTranscriptCue) => {
        if (alignMode) {
            const nextOffset = Math.round(currentTime * 1000) - cue.startMs;
            setPausedAtSec(null);
            setAlignMode(false);
            setAutoFollow(true);
            updateOffsetMs(nextOffset);
            return;
        }
        setAutoFollow(true);
        onSeek(Math.max(0, (cue.startMs + offsetMs) / 1000));
    };

    if (cues.length === 0) {
        return (
            <div className="flex-1 flex flex-col items-center justify-center text-text-subtle opacity-70 text-center py-10 px-4">
                <MessageSquareText size={40} className="mb-3 opacity-60" aria-hidden="true" />
                <p className="text-sm">{errorMessage || 'No transcript available for this episode.'}</p>
            </div>
        );
    }

    const syncToolbar = (
        <div
            className={`flex flex-wrap items-center justify-between gap-1.5 rounded-xl bg-surface-2/80 border border-surface-border px-2.5 py-1.5 text-[11px] shrink-0 ${
                onStage ? 'mx-auto w-full max-w-2xl mt-3' : ''
            }`}
        >
            <div className="flex flex-wrap items-center gap-1.5">
                <button
                    type="button"
                    onClick={handleToggleBreakPause}
                    aria-pressed={pausedAtSec !== null}
                    title={
                        pausedAtSec !== null
                            ? 'Resume transcript and shift timing by the sponsor break duration'
                            : 'Freeze transcript during a sponsor break; resuming shifts timing automatically'
                    }
                    className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 font-semibold transition-colors ${
                        pausedAtSec !== null
                            ? 'bg-brand text-surface-0'
                            : 'bg-surface-3 text-text-main hover:bg-surface-highlight'
                    }`}
                >
                    {pausedAtSec !== null ? (
                        <>
                            <Play size={11} aria-hidden="true" />
                            <span>
                                Resume (+{formatTimestamp(Math.max(0, currentTime - pausedAtSec))})
                            </span>
                        </>
                    ) : (
                        <>
                            <Pause size={11} aria-hidden="true" />
                            <span>Pause for break</span>
                        </>
                    )}
                </button>

                <button
                    type="button"
                    onClick={() => setAlignMode((v) => !v)}
                    aria-pressed={alignMode}
                    title="Click any transcript line to align it to the current audio position"
                    className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 font-semibold transition-colors ${
                        alignMode
                            ? 'bg-brand text-surface-0'
                            : 'bg-surface-3 text-text-secondary hover:text-text-main hover:bg-surface-highlight'
                    }`}
                >
                    <Crosshair size={11} aria-hidden="true" />
                    <span>{alignMode ? 'Click spoken line…' : 'Align line'}</span>
                </button>

                {nonContentChapter && nonContentChapter.endTime !== undefined && (
                    <button
                        type="button"
                        onClick={() => {
                            if (nonContentChapter.endTime !== undefined) {
                                onSeek(nonContentChapter.endTime);
                            }
                        }}
                        className="inline-flex items-center gap-1 rounded-full bg-warning/15 text-warning px-2.5 py-1 font-semibold hover:bg-warning/25 transition-colors"
                    >
                        <FastForward size={11} aria-hidden="true" />
                        <span>Skip break</span>
                    </button>
                )}
            </div>

            <div className="flex items-center gap-1 font-mono">
                <button
                    type="button"
                    onClick={() => updateOffsetMs(offsetMs - 5000)}
                    aria-label="Shift transcript earlier by 5 seconds"
                    className="rounded px-1.5 py-0.5 bg-surface-3 text-text-secondary hover:text-text-main hover:bg-surface-highlight"
                >
                    −5s
                </button>
                <button
                    type="button"
                    onClick={() => updateOffsetMs(offsetMs - 1000)}
                    aria-label="Shift transcript earlier by 1 second"
                    className="rounded p-1 bg-surface-3 text-text-secondary hover:text-text-main hover:bg-surface-highlight"
                >
                    <Minus size={10} aria-hidden="true" />
                </button>
                <button
                    type="button"
                    onClick={() => updateOffsetMs(0)}
                    disabled={offsetMs === 0}
                    title="Transcript sync offset (click to reset to 0s)"
                    aria-label={`Transcript offset ${formatOffsetLabel(offsetMs)}, click to reset`}
                    className={`inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-semibold ${
                        offsetMs !== 0
                            ? 'bg-brand/15 text-brand hover:bg-brand/25'
                            : 'text-text-subtle'
                    }`}
                >
                    <span>{formatOffsetLabel(offsetMs)}</span>
                    {offsetMs !== 0 && <RotateCcw size={10} aria-hidden="true" />}
                </button>
                <button
                    type="button"
                    onClick={() => updateOffsetMs(offsetMs + 1000)}
                    aria-label="Shift transcript later by 1 second"
                    className="rounded p-1 bg-surface-3 text-text-secondary hover:text-text-main hover:bg-surface-highlight"
                >
                    <Plus size={10} aria-hidden="true" />
                </button>
                <button
                    type="button"
                    onClick={() => updateOffsetMs(offsetMs + 5000)}
                    aria-label="Shift transcript later by 5 seconds"
                    className="rounded px-1.5 py-0.5 bg-surface-3 text-text-secondary hover:text-text-main hover:bg-surface-highlight"
                >
                    +5s
                </button>
            </div>
        </div>
    );

    if (trimmedQuery) {
        return (
            <div className="flex-1 min-h-0 flex flex-col gap-2">
                {syncToolbar}
                {filteredCues.length === 0 ? (
                    <div className="rounded-xl bg-surface-2/60 border border-surface-border p-6 text-center text-xs text-text-subtle">
                        No transcript lines match &ldquo;{query.trim()}&rdquo;.
                    </div>
                ) : (
                    <div className="flex-1 min-h-0 overflow-y-auto space-y-2 pr-1" aria-label="Transcript search results">
                        {filteredCues.map((cue, idx) => {
                            const isActive = cue === activeCue;
                            const displaySeconds = Math.max(0, (cue.startMs + offsetMs) / 1000);
                            return (
                                <button
                                    key={`${cue.startMs}-${idx}`}
                                    type="button"
                                    onClick={() => handleCueAction(cue)}
                                    aria-current={isActive ? 'true' : undefined}
                                    className={`w-full flex items-start gap-2.5 p-2.5 rounded-xl border text-left transition-colors ${
                                        isActive
                                            ? 'bg-brand/15 border-brand/40 text-text-main'
                                            : 'bg-surface-2/70 border-surface-border text-text-secondary hover:bg-surface-hover hover:text-text-main'
                                    }`}
                                >
                                    <span className={`text-[11px] font-mono shrink-0 pt-0.5 ${isActive ? 'text-brand font-semibold' : 'text-text-subtle'}`}>
                                        {formatTimestamp(displaySeconds)}
                                    </span>
                                    <div className="flex-1 min-w-0 text-xs leading-relaxed">
                                        {cue.speaker && (
                                            <span className="font-semibold text-text-main mr-1.5">
                                                {cue.speaker}:
                                            </span>
                                        )}
                                        <span className={isActive ? 'text-text-main' : 'text-text-secondary'}>
                                            {cue.text}
                                        </span>
                                    </div>
                                </button>
                            );
                        })}
                    </div>
                )}
            </div>
        );
    }

    return (
        <div className="relative flex-1 min-h-0 flex flex-col gap-2">
            {syncToolbar}

            <div
                ref={viewportRef}
                data-transcript-scroll
                onWheel={pauseAutoFollowTemporarily}
                onTouchStart={pauseAutoFollowTemporarily}
                className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden overscroll-contain [mask-image:linear-gradient(to_bottom,transparent,black_10%,black_88%,transparent)]"
                aria-label="Scrolling episode transcript"
            >
                <div className={`space-y-6 py-[42%] text-center ${onStage ? 'mx-auto w-full max-w-4xl px-3 sm:px-8' : ''}`}>
                    {cues.map((cue, idx) => {
                        const isActive = idx === activeCueIndex;
                        const prevSpeaker = idx > 0 ? cues[idx - 1]?.speaker : '';
                        const showSpeaker = Boolean(cue.speaker && cue.speaker !== prevSpeaker);
                        const startSeconds = Math.max(0, (cue.startMs + offsetMs) / 1000);
                        return (
                            <div
                                key={`${cue.startMs}-${idx}`}
                                ref={isActive ? activeLineRef : null}
                                role="button"
                                tabIndex={0}
                                aria-current={isActive ? 'true' : undefined}
                                title={
                                    alignMode
                                        ? 'Align transcript to this line'
                                        : `Jump to ${formatTimestamp(startSeconds)}`
                                }
                                className={`cursor-pointer transition-all duration-500 font-bold leading-snug px-4 motion-reduce:transition-none ${
                                    onStage
                                        ? 'text-[clamp(1.35rem,4vw,3.5rem)] tracking-tight'
                                        : 'text-card md:text-section'
                                } ${
                                    isActive
                                        ? `scale-105 origin-center motion-reduce:transform-none ${
                                              onStage
                                                  ? 'bg-gradient-to-r from-text-main via-text-main to-brand bg-clip-text text-transparent'
                                                  : 'text-text-main'
                                          }`
                                        : alignMode
                                          ? 'text-text-main opacity-65 hover:opacity-100 hover:text-brand'
                                          : 'text-text-main opacity-30 blur-[1px] hover:opacity-60 hover:blur-0'
                                }`}
                                onClick={() => handleCueAction(cue)}
                                onKeyDown={(e) => {
                                    if (e.key === 'Enter' || e.key === ' ') {
                                        e.preventDefault();
                                        e.stopPropagation();
                                        handleCueAction(cue);
                                    }
                                }}
                            >
                                {showSpeaker && (
                                    <span
                                        className={`block text-[11px] font-bold uppercase tracking-widest mb-1 ${
                                            isActive ? 'text-brand' : 'text-text-subtle'
                                        }`}
                                    >
                                        {cue.speaker}
                                    </span>
                                )}
                                <span>{cue.text}</span>
                            </div>
                        );
                    })}
                </div>
            </div>

            {!autoFollow && activeCueIndex >= 0 && (
                <div className="pointer-events-none absolute inset-x-0 bottom-3 flex justify-center">
                    <button
                        type="button"
                        onClick={() => {
                            if (resumeTimerRef.current !== null) {
                                window.clearTimeout(resumeTimerRef.current);
                                resumeTimerRef.current = null;
                            }
                            setAutoFollow(true);
                            window.requestAnimationFrame(() => centerActiveLine(false));
                        }}
                        className="pointer-events-auto inline-flex items-center gap-1.5 rounded-full bg-surface-2/95 border border-surface-border px-3 py-1.5 text-xs font-semibold text-brand shadow-lg backdrop-blur-md hover:bg-surface-3 transition-colors"
                    >
                        <LocateFixed size={13} aria-hidden="true" />
                        Sync to current line
                    </button>
                </div>
            )}
        </div>
    );
};

/** Full-stage transcript view rendered in Now Playing Transcript Focus mode. */
export const PodcastTranscriptStage: React.FC<{
    song: Song;
    currentTime: number;
    onSeek: (time: number) => void;
    isVisible: boolean;
}> = ({ song, currentTime, onSeek, isVisible }) => {
    const episodeId = song.podcastEpisodeId;
    const [cues, setCues] = useState<PodcastTranscriptCue[]>([]);
    const [chapters, setChapters] = useState<PodcastChapter[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        setCues([]);
        setChapters([]);
        setError(null);
        if (!episodeId) return;
        const controller = new AbortController();
        setLoading(true);
        void Promise.allSettled([
            podcastsAPI.getEpisodeTranscript(episodeId, controller.signal),
            podcastsAPI.getEpisodeChapters(episodeId, controller.signal),
        ]).then(([transcriptRes, chaptersRes]) => {
            if (controller.signal.aborted) return;
            if (transcriptRes.status === 'fulfilled') {
                setCues(transcriptRes.value.cues ?? []);
            } else {
                setError('Transcript not available for this episode.');
            }
            if (chaptersRes.status === 'fulfilled') {
                setChapters(chaptersRes.value.chapters ?? []);
            }
            setLoading(false);
        });
        return () => controller.abort();
    }, [episodeId]);

    if (loading) {
        return (
            <div className="flex-1 flex items-center justify-center text-text-subtle opacity-70 animate-pulse">
                Loading transcript...
            </div>
        );
    }

    return (
        <PodcastTranscriptLyrics
            episodeId={episodeId}
            cues={cues}
            chapters={chapters}
            currentTime={currentTime}
            onSeek={onSeek}
            isVisible={isVisible}
            presentation="stage"
            errorMessage={error}
        />
    );
};

export const PodcastDrawerPanel: React.FC<PodcastDrawerPanelProps> = ({
    song,
    currentTime,
    onSeek,
    onNavigate,
    isVisible = true,
}) => {
    const episodeId = song.podcastEpisodeId;
    const showToast = useStore((state) => state.showToast);

    const [subTab, setSubTab] = useState<PodcastSubTab>('chapters');
    const [loading, setLoading] = useState(false);
    const [detail, setDetail] = useState<PodcastEpisodeDetail | null>(null);
    const [chapters, setChapters] = useState<PodcastChapter[]>([]);
    const [cues, setCues] = useState<PodcastTranscriptCue[]>([]);
    const [bookmarks, setBookmarks] = useState<PodcastBookmark[]>([]);
    const [chaptersError, setChaptersError] = useState<string | null>(null);
    const [transcriptError, setTranscriptError] = useState<string | null>(null);
    const [bookmarksError, setBookmarksError] = useState<string | null>(null);

    const [transcriptQuery, setTranscriptQuery] = useState('');
    const [bookmarkNote, setBookmarkNote] = useState('');
    const [isSavingBookmark, setIsSavingBookmark] = useState(false);
    const [deletingBookmarkId, setDeletingBookmarkId] = useState<string | null>(null);

    useEffect(() => {
        setDetail(null);
        setChapters([]);
        setCues([]);
        setBookmarks([]);
        setChaptersError(null);
        setTranscriptError(null);
        setBookmarksError(null);
        setTranscriptQuery('');
        setBookmarkNote('');

        if (!episodeId) return;

        const controller = new AbortController();
        const { signal } = controller;
        setLoading(true);

        void Promise.allSettled([
            podcastsAPI.getEpisodeDetail(episodeId, signal),
            podcastsAPI.getEpisodeChapters(episodeId, signal),
            podcastsAPI.getEpisodeTranscript(episodeId, signal),
            podcastsAPI.listBookmarks(episodeId, signal),
        ]).then(([detailRes, chaptersRes, transcriptRes, bookmarksRes]) => {
            if (signal.aborted) return;

            if (detailRes.status === 'fulfilled') {
                setDetail(detailRes.value.episode ?? null);
            }

            if (chaptersRes.status === 'fulfilled') {
                setChapters(chaptersRes.value.chapters ?? []);
            } else {
                setChaptersError('Chapters unavailable');
            }

            if (transcriptRes.status === 'fulfilled') {
                setCues(transcriptRes.value.cues ?? []);
            } else {
                setTranscriptError('Transcript unavailable');
            }

            if (bookmarksRes.status === 'fulfilled') {
                setBookmarks(sortBookmarks(bookmarksRes.value.bookmarks ?? []));
            } else {
                setBookmarksError('Bookmarks unavailable');
            }

            setLoading(false);
        });

        return () => controller.abort();
    }, [episodeId]);

    useEffect(() => {
        if (!episodeId) return;
        const onBookmarkCreated = (event: Event) => {
            const custom = event as CustomEvent<{ episodeId?: string; bookmark?: PodcastBookmark }>;
            if (custom.detail?.episodeId !== episodeId || !custom.detail.bookmark) return;
            const created = custom.detail.bookmark;
            setBookmarks((prev) => {
                if (prev.some((b) => b.id === created.id)) return prev;
                return sortBookmarks([...prev, created]);
            });
        };
        window.addEventListener('viib:podcast-bookmark-created', onBookmarkCreated);
        return () => window.removeEventListener('viib:podcast-bookmark-created', onBookmarkCreated);
    }, [episodeId]);

    const activeChapterIndex = useMemo(() => {
        return chapters.reduce((best, ch, idx) => {
            if (currentTime >= ch.startTime && (ch.endTime === undefined || currentTime < ch.endTime)) {
                return idx;
            }
            return best;
        }, -1);
    }, [chapters, currentTime]);

    const currentTimeMs = currentTime * 1000;
    const activeCueIndex = useMemo(() => {
        return cues.reduce((best, cue, idx) => {
            if (currentTimeMs >= cue.startMs && (cue.endMs <= cue.startMs || currentTimeMs < cue.endMs)) {
                return idx;
            }
            return best;
        }, -1);
    }, [cues, currentTimeMs]);

    const activeCue = activeCueIndex >= 0 ? cues[activeCueIndex] : null;

    const filteredCues = useMemo(() => {
        const q = transcriptQuery.trim().toLowerCase();
        if (!q) return cues;
        return cues.filter(
            (cue) =>
                cue.text.toLowerCase().includes(q) ||
                (cue.speaker ? cue.speaker.toLowerCase().includes(q) : false),
        );
    }, [cues, transcriptQuery]);

    if (!episodeId) {
        return (
            <div className="flex-1 flex flex-col items-center justify-center text-center text-text-subtle py-12">
                <BookOpen size={36} className="mb-3 opacity-60" aria-hidden="true" />
                <p className="text-sm">Podcast episode details are unavailable.</p>
            </div>
        );
    }

    const canPrevChapter =
        chapters.length > 0 &&
        (activeChapterIndex > 0 ||
            (activeChapterIndex === 0 && currentTime > chapters[0].startTime + 2));

    const nextChapterIndex =
        activeChapterIndex >= 0
            ? activeChapterIndex + 1
            : chapters.findIndex((ch) => ch.startTime > currentTime);
    const canNextChapter =
        chapters.length > 0 && nextChapterIndex >= 0 && nextChapterIndex < chapters.length;

    const handlePrevChapter = () => {
        if (!canPrevChapter) return;
        if (activeChapterIndex > 0) {
            onSeek(chapters[activeChapterIndex - 1].startTime);
        } else {
            onSeek(chapters[0].startTime);
        }
    };

    const handleNextChapter = () => {
        if (!canNextChapter) return;
        onSeek(chapters[nextChapterIndex].startTime);
    };

    const handleCreateBookmark = async (e?: React.FormEvent) => {
        e?.preventDefault();
        if (!episodeId || isSavingBookmark) return;
        setIsSavingBookmark(true);
        const positionMs = Math.max(0, Math.round(currentTime * 1000));
        try {
            const { bookmark } = await podcastsAPI.createBookmark(
                episodeId,
                positionMs,
                bookmarkNote.trim(),
            );
            setBookmarks((prev) =>
                prev.some((b) => b.id === bookmark.id) ? prev : sortBookmarks([...prev, bookmark]),
            );
            setBookmarkNote('');
            setBookmarksError(null);
            showToast({
                type: 'success',
                message: `Bookmarked at ${formatTimestamp(positionMs / 1000)}`,
                duration: 2500,
            });
        } catch {
            showToast({
                type: 'error',
                message: 'Failed to save bookmark',
            });
        } finally {
            setIsSavingBookmark(false);
        }
    };

    const handleDeleteBookmark = async (bookmarkId: string) => {
        if (!episodeId || deletingBookmarkId) return;
        setDeletingBookmarkId(bookmarkId);
        try {
            await podcastsAPI.deleteBookmark(episodeId, bookmarkId);
            setBookmarks((prev) => prev.filter((b) => b.id !== bookmarkId));
            showToast({
                type: 'info',
                message: 'Bookmark removed',
                duration: 2000,
            });
        } catch {
            showToast({
                type: 'error',
                message: 'Failed to remove bookmark',
            });
        } finally {
            setDeletingBookmarkId(null);
        }
    };

    const subTabs: Array<{ id: PodcastSubTab; label: string; icon: React.ReactNode; count?: number }> = [
        { id: 'chapters', label: 'Chapters', icon: <ListOrdered size={13} />, count: chapters.length },
        { id: 'transcript', label: 'Transcript', icon: <MessageSquareText size={13} />, count: cues.length },
        { id: 'bookmarks', label: 'Bookmarks', icon: <Bookmark size={13} />, count: bookmarks.length },
        { id: 'notes', label: 'Notes', icon: <FileText size={13} /> },
    ];

    return (
        <div className="flex flex-col h-full min-h-0 gap-3 text-text-main">
            {/* Compact Sub-tabs */}
            <div
                role="tablist"
                aria-label="Podcast episode sections"
                className="grid grid-cols-4 gap-1 rounded-xl bg-surface-2 p-1 border border-surface-border shrink-0"
            >
                {subTabs.map((tab) => {
                    const selected = subTab === tab.id;
                    const compactCount =
                        tab.count !== undefined && tab.count > 0
                            ? tab.count > 99
                                ? '99+'
                                : String(tab.count)
                            : null;
                    return (
                        <button
                            key={tab.id}
                            type="button"
                            role="tab"
                            aria-selected={selected}
                            title={
                                tab.count !== undefined && tab.count > 0
                                    ? `${tab.label} (${tab.count.toLocaleString()})`
                                    : tab.label
                            }
                            onClick={() => setSubTab(tab.id)}
                            className={`flex items-center justify-center gap-1 rounded-lg px-1.5 py-1.5 text-xs font-medium transition-colors whitespace-nowrap ${
                                selected
                                    ? 'bg-surface-3 text-text-main shadow-sm'
                                    : 'text-text-secondary hover:text-text-main hover:bg-surface-hover'
                            }`}
                        >
                            <span aria-hidden="true" className="shrink-0">
                                {tab.icon}
                            </span>
                            <span>{tab.label}</span>
                            {compactCount && tab.id !== 'transcript' && (
                                <span className="hidden xl:inline text-[10px] font-mono text-text-subtle">
                                    ({compactCount})
                                </span>
                            )}
                        </button>
                    );
                })}
            </div>

            {/* Sub-tab Body */}
            <div className={`flex-1 min-h-0 ${subTab === 'transcript' ? 'flex flex-col overflow-hidden' : 'overflow-y-auto pr-1'}`}>
                {loading && (
                    <div className="flex items-center justify-center gap-2 py-8 text-xs text-text-subtle">
                        <Loader2 size={15} className="animate-spin" aria-hidden="true" />
                        <span>Loading episode details...</span>
                    </div>
                )}

                {!loading && subTab === 'chapters' && (
                    <div className="space-y-3">
                        {chapters.length > 0 && (
                            <div className="flex items-center justify-between gap-2 p-2 rounded-xl bg-surface-2 border border-surface-border">
                                <Button
                                    variant="secondary"
                                    size="sm"
                                    onClick={handlePrevChapter}
                                    disabled={!canPrevChapter}
                                    leftIcon={<ChevronLeft size={14} aria-hidden="true" />}
                                    aria-label="Previous chapter"
                                >
                                    Prev Chapter
                                </Button>
                                <span className="text-[11px] font-mono text-text-subtle">
                                    {activeChapterIndex >= 0
                                        ? `${activeChapterIndex + 1} / ${chapters.length}`
                                        : `${chapters.length} chapters`}
                                </span>
                                <Button
                                    variant="secondary"
                                    size="sm"
                                    onClick={handleNextChapter}
                                    disabled={!canNextChapter}
                                    rightIcon={<ChevronRight size={14} aria-hidden="true" />}
                                    aria-label="Next chapter"
                                >
                                    Next Chapter
                                </Button>
                            </div>
                        )}

                        {chapters.length === 0 ? (
                            <div className="rounded-xl bg-surface-2/60 border border-surface-border p-6 text-center text-xs text-text-subtle">
                                {chaptersError || 'No chapters available for this episode.'}
                            </div>
                        ) : (
                            <ul className="space-y-1.5" aria-label="Episode chapters">
                                {chapters.map((chapter, idx) => {
                                    const isActive = idx === activeChapterIndex;
                                    const safeImg = safeChapterImageURL(chapter.img);
                                    return (
                                        <li key={`${chapter.startTime}-${idx}`}>
                                            <button
                                                type="button"
                                                onClick={() => onSeek(chapter.startTime)}
                                                aria-current={isActive ? 'true' : undefined}
                                                className={`w-full flex items-center gap-3 p-2.5 rounded-xl border text-left transition-colors ${
                                                    isActive
                                                        ? 'bg-brand/15 border-brand/40 text-text-main'
                                                        : 'bg-surface-2/70 border-surface-border text-text-secondary hover:bg-surface-hover hover:text-text-main'
                                                }`}
                                            >
                                                {safeImg && (
                                                    <img
                                                        src={safeImg}
                                                        alt=""
                                                        className="w-9 h-9 rounded-md object-cover bg-surface-3 shrink-0"
                                                    />
                                                )}
                                                <div className="flex-1 min-w-0">
                                                    <div className={`text-xs font-semibold truncate ${isActive ? 'text-brand' : 'text-text-main'}`}>
                                                        {chapter.title || `Chapter ${idx + 1}`}
                                                    </div>
                                                    <div className="text-[11px] font-mono text-text-subtle">
                                                        {formatTimestamp(chapter.startTime)}
                                                    </div>
                                                </div>
                                                {chapter.url && chapter.url.startsWith('https://') && (
                                                    <a
                                                        href={chapter.url}
                                                        target="_blank"
                                                        rel="noopener noreferrer"
                                                        onClick={(e) => e.stopPropagation()}
                                                        aria-label={`Open link for ${chapter.title || 'chapter'}`}
                                                        className="p-1.5 rounded-full text-text-subtle hover:text-brand hover:bg-surface-3 shrink-0"
                                                    >
                                                        <ExternalLink size={13} />
                                                    </a>
                                                )}
                                            </button>
                                        </li>
                                    );
                                })}
                            </ul>
                        )}
                    </div>
                )}

                {!loading && subTab === 'transcript' && (
                    <div className="flex-1 min-h-0 flex flex-col gap-3">
                        {cues.length > 0 && (
                            <div className="relative shrink-0">
                                <Search
                                    size={14}
                                    aria-hidden="true"
                                    className="absolute left-3 top-1/2 -translate-y-1/2 text-text-subtle pointer-events-none"
                                />
                                <input
                                    type="search"
                                    value={transcriptQuery}
                                    onChange={(e) => setTranscriptQuery(e.target.value)}
                                    placeholder={`Search ${cues.length.toLocaleString()} transcript lines...`}
                                    aria-label="Search transcript"
                                    className="w-full rounded-xl bg-surface-2 border border-surface-border pl-8 pr-8 py-2 text-xs text-text-main placeholder:text-text-subtle outline-none focus:ring-2 focus:ring-brand"
                                />
                                {transcriptQuery && (
                                    <button
                                        type="button"
                                        onClick={() => setTranscriptQuery('')}
                                        aria-label="Clear transcript search"
                                        className="absolute right-2.5 top-1/2 -translate-y-1/2 p-0.5 rounded-full text-text-subtle hover:text-text-main"
                                    >
                                        <X size={13} />
                                    </button>
                                )}
                            </div>
                        )}

                        <PodcastTranscriptLyrics
                            episodeId={episodeId}
                            cues={cues}
                            chapters={chapters}
                            currentTime={currentTime}
                            onSeek={onSeek}
                            isVisible={isVisible && subTab === 'transcript'}
                            presentation="drawer"
                            query={transcriptQuery}
                            errorMessage={transcriptError}
                        />
                    </div>
                )}

                {!loading && subTab === 'bookmarks' && (
                    <div className="space-y-3">
                        <form
                            onSubmit={handleCreateBookmark}
                            className="p-3 rounded-xl bg-surface-2 border border-surface-border space-y-2.5"
                        >
                            <input
                                type="text"
                                value={bookmarkNote}
                                onChange={(e) => setBookmarkNote(e.target.value)}
                                maxLength={500}
                                placeholder="Optional note for this moment..."
                                aria-label="Bookmark note"
                                className="w-full rounded-lg bg-surface-3 border border-surface-border px-3 py-1.5 text-xs text-text-main placeholder:text-text-subtle outline-none focus:ring-2 focus:ring-brand"
                            />
                            <Button
                                type="submit"
                                variant="primary"
                                size="sm"
                                disabled={isSavingBookmark}
                                leftIcon={
                                    isSavingBookmark ? (
                                        <Loader2 size={13} className="animate-spin" aria-hidden="true" />
                                    ) : (
                                        <Bookmark size={13} aria-hidden="true" />
                                    )
                                }
                                className="w-full"
                            >
                                Bookmark current moment ({formatTimestamp(currentTime)})
                            </Button>
                        </form>

                        {bookmarksError && bookmarks.length === 0 ? (
                            <div className="rounded-xl bg-surface-2/60 border border-surface-border p-6 text-center text-xs text-text-subtle">
                                {bookmarksError}
                            </div>
                        ) : bookmarks.length === 0 ? (
                            <div className="rounded-xl bg-surface-2/60 border border-surface-border p-6 text-center text-xs text-text-subtle">
                                No bookmarks yet. Save a moment while listening.
                            </div>
                        ) : (
                            <ul className="space-y-1.5" aria-label="Episode bookmarks">
                                {bookmarks.map((bm) => {
                                    const posSeconds = bm.positionMs / 1000;
                                    const isDeleting = deletingBookmarkId === bm.id;
                                    return (
                                        <li
                                            key={bm.id}
                                            className="flex items-center gap-2 p-2 rounded-xl bg-surface-2/70 border border-surface-border hover:bg-surface-hover transition-colors"
                                        >
                                            <button
                                                type="button"
                                                onClick={() => onSeek(posSeconds)}
                                                className="flex-1 min-w-0 flex items-start gap-2.5 text-left"
                                            >
                                                <span className="text-[11px] font-mono text-brand font-semibold shrink-0 pt-0.5">
                                                    {formatTimestamp(posSeconds)}
                                                </span>
                                                <span className="text-xs text-text-main break-words">
                                                    {bm.note || 'Bookmarked moment'}
                                                </span>
                                            </button>
                                            <button
                                                type="button"
                                                onClick={() => void handleDeleteBookmark(bm.id)}
                                                disabled={isDeleting}
                                                aria-label={`Delete bookmark at ${formatTimestamp(posSeconds)}`}
                                                title="Delete bookmark"
                                                className="p-1.5 rounded-full text-text-subtle hover:text-error hover:bg-surface-3 transition-colors shrink-0 disabled:opacity-50"
                                            >
                                                {isDeleting ? (
                                                    <Loader2 size={13} className="animate-spin" />
                                                ) : (
                                                    <Trash2 size={13} />
                                                )}
                                            </button>
                                        </li>
                                    );
                                })}
                            </ul>
                        )}
                    </div>
                )}

                {!loading && subTab === 'notes' && (
                    <div className="space-y-3">
                        <div className="p-3.5 rounded-xl bg-surface-2 border border-surface-border flex items-start justify-between gap-3">
                            <div className="min-w-0">
                                <div className="text-xs font-semibold text-text-main truncate">
                                    {detail?.feedTitle || song.album}
                                </div>
                                {(detail?.feedAuthor || song.artist) && (
                                    <div className="text-[11px] text-text-secondary truncate">
                                        {detail?.feedAuthor || song.artist}
                                    </div>
                                )}
                            </div>
                            <EpisodeNavButton
                                episodeId={episodeId}
                                onNavigate={onNavigate}
                                className="inline-flex items-center gap-1.5 rounded-full bg-surface-3 hover:bg-surface-highlight px-3 py-1.5 text-xs font-semibold text-text-main transition-colors shrink-0"
                            >
                                <span>Open Episode</span>
                                <ExternalLink size={12} aria-hidden="true" />
                            </EpisodeNavButton>
                        </div>

                        <div className="p-4 rounded-xl bg-surface-2/60 border border-surface-border">
                            {renderSafeRichDescription(detail?.richDescription, detail?.description)}
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
};
