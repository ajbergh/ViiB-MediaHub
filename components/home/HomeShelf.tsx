/** Lays out a titled horizontal content shelf on Home. */

import React from 'react';
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
  return (
    <section className={cn('mb-8', className)}>
      <div className="mb-3 flex items-end justify-between gap-4">
        <div className="min-w-0">
          {subtitle ? <p className="mb-0.5 text-xs font-medium text-text-secondary">{subtitle}</p> : null}
          <h2 className="text-section font-bold tracking-tight text-text-main">{title}</h2>
        </div>
        {actionLabel && onAction ? (
          <button
            type="button"
            onClick={onAction}
            className="flex-shrink-0 text-xs font-bold uppercase tracking-wider text-text-secondary transition-colors hover:text-text-main hover:underline"
          >
            Show all
          </button>
        ) : null}
      </div>

      <div className={cn('-mx-3 flex gap-1 overflow-x-auto px-3 pb-2 scrollbar-hide', contentClassName)}>
        {children}
      </div>
    </section>
  );
};
