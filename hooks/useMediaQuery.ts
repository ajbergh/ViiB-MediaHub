import { useEffect, useState } from 'react';

/**
 * Subscribe to a CSS media query and return whether it currently matches.
 * SSR-safe: returns `false` until the first effect runs in the browser.
 */
export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
      return false;
    }
    return window.matchMedia(query).matches;
  });

  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
      return;
    }
    const mql = window.matchMedia(query);
    const onChange = (e: MediaQueryListEvent) => setMatches(e.matches);
    setMatches(mql.matches);
    mql.addEventListener('change', onChange);
    return () => mql.removeEventListener('change', onChange);
  }, [query]);

  return matches;
}

export const BREAKPOINTS = {
  sm: '(min-width: 640px)',
  md: '(min-width: 768px)',
  lg: '(min-width: 1024px)',
  xl: '(min-width: 1280px)',
  dj: '(min-width: 1440px)',
  dj1080p: '(min-width: 1920px) and (min-height: 1080px)',
} as const;

export const useIsMobile = () => !useMediaQuery(BREAKPOINTS.md);
export const useIsDJReady = () => useMediaQuery(BREAKPOINTS.dj);

/**
 * Returns true when the viewport is at least 1920×1080 (full HD) OR
 * the document is currently in fullscreen mode.
 * DJ Mode v2 is designed for this resolution; callers should offer a
 * "go fullscreen" prompt when this returns false.
 */
export function useIsDJ1080p(): boolean {
  const matchesMQ = useMediaQuery(BREAKPOINTS.dj1080p);
  const [isFS, setIsFS] = useState(() =>
    typeof document !== 'undefined' ? !!document.fullscreenElement : false
  );

  useEffect(() => {
    const onFSChange = () => setIsFS(!!document.fullscreenElement);
    document.addEventListener('fullscreenchange', onFSChange);
    return () => document.removeEventListener('fullscreenchange', onFSChange);
  }, []);

  return matchesMQ || isFS;
}
