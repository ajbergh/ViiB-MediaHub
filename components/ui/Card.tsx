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
  default: 'bg-surface-2/80 p-4',
  hero: 'bg-surface-2 p-6',
  utility: 'bg-surface-2 p-4',
};

export const Card: React.FC<CardProps> = ({ 
  variant = 'default', 
  interactive = false, 
  className, 
  ...rest 
}) => {
  const base = cn(
    'rounded-lg text-text-main',
    variantStyles[variant]
  );

  const interactiveClasses =
    'transition-colors duration-150 ease-out ' +
    'hover:bg-surface-hover ' +
    'motion-reduce:transition-none';

  return (
    <div
      className={cn(base, interactive && interactiveClasses, className)}
      {...rest}
    />
  );
};
