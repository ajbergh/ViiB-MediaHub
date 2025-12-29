/**
 * ViiB MediaHub - Card Component
 * 
 * A composable container component for content presentation with built-in
 * visual hierarchy through variants. Cards are the primary surface for
 * grouping related content throughout the app.
 * 
 * Features:
 * - Three semantic variants for visual hierarchy (default, hero, utility)
 * - Optional interactive state with hover animations
 * - Composable design - accepts any children for flexible content
 * - Consistent styling via design tokens (surface colors, rounded corners, rings)
 * 
 * Usage:
 * - default: Album cards, playlist cards, general content containers
 * - hero: Featured content, main call-to-action sections (e.g., Featured Mix on Home)
 * - utility: Stats display, counts, metadata cards (e.g., Home stats cards)
 * 
 * @module Card
 */

import React from 'react';
import { cn } from './cn';

/**
 * Card variants for visual hierarchy:
 * - default: Standard content cards (bg-surface-2, p-6)
 * - hero: Featured/prominent content (bg-surface-2, p-8, shadow-lg)
 * - utility: Stats, counts, metadata (bg-surface-1, p-4)
 */
export type CardVariant = 'default' | 'hero' | 'utility';

export interface CardProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: CardVariant;
  interactive?: boolean;
}

const variantStyles: Record<CardVariant, string> = {
  default: 'bg-surface-2 p-6',
  hero: 'bg-surface-2 p-8 shadow-lg',
  utility: 'bg-surface-1 p-4',
};

export const Card: React.FC<CardProps> = ({ 
  variant = 'default', 
  interactive = false, 
  className, 
  ...rest 
}) => {
  const base = cn(
    'rounded-xl ring-1 ring-surface-3/70 text-text-main',
    variantStyles[variant]
  );

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
