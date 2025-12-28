import React from 'react';
import { cn } from './cn';

export interface PageProps extends React.HTMLAttributes<HTMLDivElement> {
  withPlayerPadding?: boolean;
  withFadeIn?: boolean;
}

export const Page: React.FC<PageProps> = ({
  withPlayerPadding = true,
  withFadeIn = true,
  className,
  ...rest
}) => {
  return (
    <div
      className={cn(
        'p-8 h-full',
        withPlayerPadding && 'pb-32',
        withFadeIn && 'animate-fade-in',
        className
      )}
      {...rest}
    />
  );
};

export interface PageHeaderProps extends React.HTMLAttributes<HTMLDivElement> {
  heading: React.ReactNode;
  subtitle?: React.ReactNode;
  actions?: React.ReactNode;
  titleClassName?: string;
}

export const PageHeader: React.FC<PageHeaderProps> = ({
  heading,
  subtitle,
  actions,
  titleClassName,
  className,
  ...rest
}) => {
  return (
    <div
      className={cn(
        'mb-8 flex flex-col md:flex-row md:items-center md:justify-between gap-4',
        className
      )}
      {...rest}
    >
      <div>
        <h1 className={cn('text-display mb-2', titleClassName)}>{heading}</h1>
        {subtitle ? <p className="text-text-secondary">{subtitle}</p> : null}
      </div>

      {actions ? <div className="flex items-center gap-3">{actions}</div> : null}
    </div>
  );
};

export interface ListHeaderProps extends React.HTMLAttributes<HTMLDivElement> {}

/**
 * ListHeader
 *
 * Standard header wrapper for virtualized lists (e.g., react-virtuoso Header)
 * to keep consistent padding without repeating one-off classes.
 */
export const ListHeader: React.FC<ListHeaderProps> = ({ className, ...rest }) => {
  return <div className={cn('p-8 pb-0', className)} {...rest} />;
};
