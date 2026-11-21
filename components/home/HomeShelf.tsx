/** Lays out a titled horizontal content shelf on Home. */

import React, { useCallback, useEffect, useRef, useState } from 'react';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { cn } from '../ui/cn';

interface HomeShelfProps {
  title: string;
  subtitle?: string;
  actionLabel?: string;
  onAction?: () => void;
  children: React.ReactNode;
  className?: string;
  contentClassName?: string;
}

export const HomeShelf: React.FC<HomeShelfProps> = ({
  title,
  subtitle,
  actionLabel,
  onAction,
  children,
  className,
  contentClassName,
}) => {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [scrollState, setScrollState] = useState({ hasOverflow: false, canLeft: false, canRight: false });

  const updateScrollState = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const hasOverflow = el.scrollWidth > el.clientWidth + 1;
    const canLeft = hasOverflow && el.scrollLeft > 1;
    const canRight = hasOverflow && el.scrollLeft + el.clientWidth < el.scrollWidth - 1;
    setScrollState((prev) =>
      prev.hasOverflow === hasOverflow && prev.canLeft === canLeft && prev.canRight === canRight
        ? prev
        : { hasOverflow, canLeft, canRight }
    );
  }, []);

  useEffect(() => {
    updateScrollState();
    const el = scrollRef.current;
    if (!el) return;
    const observer = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(updateScrollState) : null;
    observer?.observe(el);
    window.addEventListener('resize', updateScrollState);
    return () => {
      observer?.disconnect();
      window.removeEventListener('resize', updateScrollState);
    };
  }, [children, updateScrollState]);

  const scrollByPage = (direction: -1 | 1) => {
    const el = scrollRef.current;
    if (!el) return;
    const step = Math.max(240, Math.round(el.clientWidth * 0.75));
    el.scrollBy({ left: direction * step, behavior: 'smooth' });
  };

  return (
    <section className={cn('mb-8', className)}>
      <div className="mb-3 flex items-end justify-between gap-4">
        <div className="min-w-0">
          {subtitle ? <p className="mb-0.5 text-xs font-medium text-text-secondary">{subtitle}</p> : null}
          <h2 className="text-section font-bold tracking-tight text-text-main">{title}</h2>
        </div>
        <div className="flex flex-shrink-0 items-center gap-1.5">
          {scrollState.hasOverflow ? (
            <div className="flex items-center gap-1">
              <button
                type="button"
                onClick={() => scrollByPage(-1)}
                disabled={!scrollState.canLeft}
                aria-label={`Previous ${title}`}
                className="inline-flex min-h-[32px] min-w-[32px] items-center justify-center rounded-full bg-surface-2 text-text-secondary transition-colors hover:bg-surface-highlight hover:text-text-main disabled:pointer-events-none disabled:opacity-40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70"
              >
                <ChevronLeft size={16} aria-hidden="true" />
              </button>
              <button
                type="button"
                onClick={() => scrollByPage(1)}
                disabled={!scrollState.canRight}
                aria-label={`Next ${title}`}
                className="inline-flex min-h-[32px] min-w-[32px] items-center justify-center rounded-full bg-surface-2 text-text-secondary transition-colors hover:bg-surface-highlight hover:text-text-main disabled:pointer-events-none disabled:opacity-40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70"
              >
                <ChevronRight size={16} aria-hidden="true" />
              </button>
            </div>
          ) : null}
          {actionLabel && onAction ? (
            <button
              type="button"
              onClick={onAction}
              className="inline-flex min-h-[32px] flex-shrink-0 items-center rounded-md px-2 text-xs font-bold uppercase tracking-wider text-text-secondary transition-colors hover:text-text-main hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70"
            >
              Show all
            </button>
          ) : null}
        </div>
      </div>

      <div
        ref={scrollRef}
        onScroll={updateScrollState}
        className={cn('-mx-3 flex gap-1 overflow-x-auto px-3 pb-2 scrollbar-hide', contentClassName)}
      >
        {children}
      </div>
    </section>
  );
};
