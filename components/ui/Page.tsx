/** Defines shared page containers, page headers, and list headers. */

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
        'px-4 py-5 sm:px-6 lg:px-7 h-full',
        withPlayerPadding && 'pb-24',
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
        'mb-6 flex flex-col md:flex-row md:items-end md:justify-between gap-4',
        className
      )}
      {...rest}
    >
      <div>
        <h1 className={cn('text-display tracking-tight text-text-main', titleClassName)}>{heading}</h1>
        {subtitle ? <p className="text-sm text-text-secondary mt-1">{subtitle}</p> : null}
      </div>

      {actions ? <div className="flex items-center gap-2.5 flex-wrap">{actions}</div> : null}
    </div>
  );
};

export interface ListHeaderProps extends React.HTMLAttributes<HTMLDivElement> {}

export const ListHeader: React.FC<ListHeaderProps> = ({ className, ...rest }) => {
  return <div className={cn('px-4 pt-5 sm:px-6 lg:px-7 pb-0', className)} {...rest} />;
};
