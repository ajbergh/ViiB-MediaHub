import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Pause, Play, RotateCcw, RotateCw, X } from 'lucide-react';
import { useStore } from '../store';
import { podcastsAPI, PodcastAPIError, type PodcastEpisode, type PodcastProgress } from '../services/podcasts';

type Props = { episode: PodcastEpisode; showTitle: string; onClose: () => void };
const rates = [0.75, 1, 1.25, 1.5, 1.75, 2] as const;

/** Early-access transport: never place a publisher enclosure URL in audio.src.
 * Existing music queue and Spotify state are not modified by podcast playback.
 * Full OS media-key session ownership remains a subsequent roadmap gate. */
export const PodcastEpisodePlayer: React.FC<Props> = ({ episode, showTitle, onClose }) => {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const progressRef = useRef<PodcastProgress | null>(null);
  const positionRef = useRef(0);
  const baseListenedRef = useRef(0);
  const sessionListenedRef = useRef(0);
  const playedRef = useRef(false);
  const lastTimeRef = useRef<number | null>(null);
  const lastSaveRef = useRef(0);
  const writingRef = useRef(false);
  const queuedRef = useRef(false);
  const conflictRef = useRef(false);
  const [ready, setReady] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [rate, setRate] = useState(1);
  const [error, setError] = useState('');

  const save = useCallback(async () => {
    if (!progressRef.current || conflictRef.current) return;
    if (writingRef.current) { queuedRef.current = true; return; }
    writingRef.current = true;
    try {
      do {
        queuedRef.current = false;
        const current = progressRef.current;
        if (!current) break;
        const positionMs = positionRef.current;
        const listenedMs = baseListenedRef.current + sessionListenedRef.current;
        const isPlayed = playedRef.current;
        // Do not increment a revision just because an unplayed player was
        // opened/closed, or because pause/cleanup triggered a duplicate save.
        if (positionMs === current.positionMs && listenedMs <= current.listenedMs &&
            isPlayed === current.isPlayed) break;
        try {
          const next = await podcastsAPI.setProgress(episode.id, {
            positionMs, listenedMs, isPlayed, expectedRevision: current.revision,
          });
          progressRef.current = next;
          lastSaveRef.current = Date.now();
          setError('');
        } catch (reason) {
          const conflict = reason instanceof PodcastAPIError && reason.status === 409;
          if (conflict) conflictRef.current = true;
          // A temporary publisher/storage error should not cause requests
          // on every audio timeupdate tick; retry at the normal interval.
          lastSaveRef.current = Date.now();
          setError(conflict
            ? 'Progress changed in another session. Reload this episode to avoid overwriting it.'
            : reason instanceof Error ? reason.message : 'Could not save listening progress.');
          break;
        }
      } while (queuedRef.current);
    } finally { writingRef.current = false; }
  }, [episode.id]);

  useEffect(() => {
    const controller = new AbortController();
    progressRef.current = null; positionRef.current = 0;
    baseListenedRef.current = 0; sessionListenedRef.current = 0;
    lastTimeRef.current = null; playedRef.current = false;
    conflictRef.current = false;
    setReady(false); setPlaying(false); setError('');
    void podcastsAPI.getProgress(episode.id, controller.signal).then(progress => {
      if (controller.signal.aborted) return;
      progressRef.current = progress;
      positionRef.current = progress.positionMs;
      baseListenedRef.current = progress.listenedMs;
      playedRef.current = progress.isPlayed;
      lastSaveRef.current = Date.now();
      setReady(true);
    }).catch(reason => {
      if (!controller.signal.aborted)
        setError(reason instanceof Error ? reason.message : 'Could not load saved progress.');
    });
    return () => {
      controller.abort();
      const audio = audioRef.current;
      if (audio && !audio.paused) audio.pause();
      void save();
    };
  }, [episode.id, save]);

  // When the music player starts, yield podcast audio without touching the
  // music queue; starting a podcast also pauses currently playing music.
  useEffect(() => useStore.subscribe((current, previous) => {
    if (current.isPlaying && !previous.isPlaying) audioRef.current?.pause();
  }), []);

  const onTime = () => {
    const audio = audioRef.current;
    if (!audio) return;
    const now = audio.currentTime;
    if (lastTimeRef.current !== null && !audio.paused && !audio.seeking) {
      const delta = now - lastTimeRef.current;
      if (delta > 0 && delta <= 15) sessionListenedRef.current += Math.round(delta * 1000);
    }
    lastTimeRef.current = now;
    positionRef.current = Math.max(0, Math.round(now * 1000));
    if (Date.now() - lastSaveRef.current >= 10000 && !writingRef.current) void save();
  };

  const onPlay = () => {
    const music = useStore.getState();
    if (music.isPlaying) music.togglePlay();
    playedRef.current = false;
    lastTimeRef.current = audioRef.current?.currentTime ?? null;
    setPlaying(true);
  };

  const toggle = async () => {
    const audio = audioRef.current;
    if (!audio || !ready) return;
    if (!audio.paused) { audio.pause(); return; }
    const music = useStore.getState();
    if (music.isPlaying) music.togglePlay();
    try { await audio.play(); }
    catch { setError('This publisher audio could not be played. Public HTTPS byte ranges are required.'); }
  };

  const seek = (delta: number) => {
    const audio = audioRef.current;
    if (!audio || !ready) return;
    const end = Number.isFinite(audio.duration) ? audio.duration : Number.MAX_SAFE_INTEGER;
    audio.currentTime = Math.max(0, Math.min(end, audio.currentTime + delta));
    positionRef.current = Math.round(audio.currentTime * 1000);
    lastTimeRef.current = audio.currentTime;
    void save();
  };

  return <section aria-label="Podcast episode player" className="rounded-xl border border-surface-3 bg-surface-2 p-5">
    <div className="flex items-start justify-between gap-3">
      <div>
        <p className="text-xs font-semibold uppercase tracking-wider text-brand">Now listening · Early access</p>
        <h2 className="mt-1 text-lg font-bold text-text-main">{episode.title}</h2>
        <p className="text-sm text-text-secondary">{showTitle}</p>
      </div>
      <button type="button" onClick={onClose} aria-label="Close podcast player"
        className="rounded-md p-2 text-text-secondary hover:bg-surface-highlight"><X size={18} /></button>
    </div>
    {!ready && !error && <p role="status" className="mt-3 text-sm text-text-secondary">Loading saved position…</p>}
    {error && <p role="alert" className="mt-3 rounded-md border border-error/30 p-3 text-sm text-error">{error}</p>}
    <audio ref={audioRef} className="mt-4 w-full" controls preload="none"
      src={ready ? podcastsAPI.mediaURL(episode.id) : undefined}
      aria-label={'Podcast audio: ' + episode.title}
      onLoadedMetadata={event => {
        const player = event.currentTarget;
        if (positionRef.current > 0) {
          const offset = positionRef.current / 1000;
          player.currentTime = Number.isFinite(player.duration)
            ? Math.min(Math.max(0, player.duration - 1), offset) : offset;
        }
        player.playbackRate = rate;
      }}
      onPlay={onPlay}
      onPause={() => { setPlaying(false); lastTimeRef.current = null; void save(); }}
      onTimeUpdate={onTime}
      onSeeking={() => { lastTimeRef.current = null; }}
      onSeeked={event => { lastTimeRef.current = event.currentTarget.currentTime; }}
      onEnded={() => { playedRef.current = true; setPlaying(false); void save(); }}
      onError={() => { if (ready) setError('Audio unavailable or incompatible with secure byte-range streaming.'); }}
    />
    <div className="mt-3 flex flex-wrap items-center gap-2">
      <button type="button" disabled={!ready} onClick={() => void toggle()}
        aria-label={playing ? 'Pause podcast' : 'Play podcast'}
        className="inline-flex items-center gap-2 rounded-md bg-brand px-3 py-2 text-sm font-semibold text-surface-0 disabled:opacity-50">
        {playing ? <Pause size={16}/> : <Play size={16}/>} {playing ? 'Pause' : 'Play'}
      </button>
      <button type="button" disabled={!ready} onClick={() => seek(-15)} aria-label="Back 15 seconds"
        className="inline-flex items-center gap-1 rounded-md border border-surface-3 px-3 py-2 text-sm text-text-main disabled:opacity-50">
        <RotateCcw size={16}/> 15s
      </button>
      <button type="button" disabled={!ready} onClick={() => seek(30)} aria-label="Forward 30 seconds"
        className="inline-flex items-center gap-1 rounded-md border border-surface-3 px-3 py-2 text-sm text-text-main disabled:opacity-50">
        <RotateCw size={16}/> 30s
      </button>
      <label htmlFor="podcast-playback-speed" className="ml-1 text-xs text-text-secondary">Speed</label>
      <select id="podcast-playback-speed" aria-label="Podcast playback speed" value={rate}
        onChange={event => {
          const next = Number(event.target.value);
          setRate(next);
          if (audioRef.current) audioRef.current.playbackRate = next;
        }}
        className="rounded-md border border-surface-3 bg-surface-1 px-2 py-2 text-sm text-text-main">
        {rates.map(speed => <option key={speed} value={speed}>{speed}×</option>)}
      </select>
    </div>
    <p className="mt-3 text-xs text-text-subtle">Streams through the local ViiB API. Progress saves periodically and on pause. OS media keys and offline audio remain planned.</p>
  </section>;
};
