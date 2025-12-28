import React from 'react';
import { cn } from './cn';

export interface CardProps extends React.HTMLAttributes<HTMLDivElement> {
  interactive?: boolean;
}

export const Card: React.FC<CardProps> = ({ interactive = false, className, ...rest }) => {
  const base =
    'rounded-xl bg-surface-1 ring-1 ring-surface-3/70 ' +
    'text-text-main';

  const interactiveClasses =
    'transition-all duration-150 ease-out ' +
    'hover:-translate-y-0.5 hover:shadow-lg hover:shadow-black/25 hover:ring-white/10 ' +
    'motion-reduce:transition-none motion-reduce:hover:transform-none';

  return (
    <div
      className={cn(base, interactive && interactiveClasses, className)}
      {...rest}
    />
  );
};
