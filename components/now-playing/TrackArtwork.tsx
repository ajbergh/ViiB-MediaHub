import { useEffect, useState } from 'react';
import { cssUrl, generateGradient } from '../../utils';

const ARTWORK_FADE_MS = 700;

interface Frame {
    id: string;
    cover?: string;
    album: string;
    initial: string;
}

interface Transition {
    current: Frame;
    previous?: Frame;
}

export function useTrackArtwork(id: string, cover: string | undefined, album: string, initial: string) {
    const [transition, setTransition] = useState<Transition>(() => ({ current: { id, cover, album, initial } }));

    useEffect(() => {
        let cancelled = false;
        const reveal = (nextCover?: string) => {
            if (cancelled) return;
            setTransition(state => {
                const { current } = state;
                if (current.id === id && current.cover === nextCover && current.album === album && current.initial === initial) {
                    return state;
                }
                const sameImage = !!nextCover && current.cover === nextCover;
                return {
                    current: { id, cover: nextCover, album, initial },
                    previous: sameImage ? undefined : current,
                };
            });
        };
        if (!cover) {
            reveal();
        } else {
            const image = new Image();
            image.onload = () => reveal(cover);
            image.onerror = () => reveal();
            image.src = cover;
        }
        return () => { cancelled = true; };
    }, [id, cover, album, initial]);

    useEffect(() => {
        if (!transition.previous) return;
        const timeout = window.setTimeout(() => {
            setTransition(state => state === transition ? { current: state.current } : state);
        }, ARTWORK_FADE_MS);
        return () => window.clearTimeout(timeout);
    }, [transition]);

    return transition;
}

export function TrackArtwork({ transition, backdrop = false }: { transition: Transition; backdrop?: boolean }) {
    const frames = transition.previous ? [transition.previous, transition.current] : [transition.current];
    return frames.map(frame => {
        const incoming = frame === transition.current;
        const fadingIn = incoming && !!transition.previous;
        return (
            <div
                key={JSON.stringify([frame.id, frame.cover, frame.album, frame.initial])}
                aria-hidden={backdrop || !incoming}
                className={`absolute inset-0 bg-surface-2 ${fadingIn ? 'motion-safe:animate-fade-in' : ''}`}
                style={{
                    animationDuration: fadingIn ? `${ARTWORK_FADE_MS}ms` : undefined,
                    background: backdrop && frame.cover
                        ? `${cssUrl(frame.cover)} center/cover`
                        : frame.cover ? undefined : generateGradient(frame.album),
                }}
            >
                {!backdrop && (frame.cover ? (
                    <img src={frame.cover} alt={incoming ? 'Album Art' : ''} className="h-full w-full object-contain" draggable={false} />
                ) : (
                    <div className="flex h-full items-center justify-center text-display font-bold text-text-main/35">{frame.initial}</div>
                ))}
            </div>
        );
    });
}
