import React from 'react';
import { ChevronRight } from 'lucide-react';
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
    <section className={cn('mb-10', className)}>
      <div className="mb-4 flex items-end justify-between gap-4">
        <div className="min-w-0">
          <h2 className="text-section font-semibold text-text-main">{title}</h2>
          {subtitle ? <p className="mt-1 text-sm text-text-subtle">{subtitle}</p> : null}
        </div>
        {actionLabel && onAction ? (
          <button
            type="button"
            onClick={onAction}
            className="inline-flex flex-shrink-0 items-center gap-1 rounded-full px-3 py-1.5 text-sm font-medium text-text-secondary transition-colors hover:bg-surface-2 hover:text-text-main"
          >
            {actionLabel}
            <ChevronRight size={15} aria-hidden="true" />
          </button>
        ) : null}
      </div>

      <div className={cn('flex gap-5 overflow-x-auto pb-3 scrollbar-hide', contentClassName)}>
        {children}
      </div>
    </section>
  );
};
