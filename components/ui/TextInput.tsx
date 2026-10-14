/** Renders the shared styled text input. */

import React from 'react';
import { cn } from './cn';

export interface TextInputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
  inputClassName?: string;
}

export const TextInput: React.FC<TextInputProps> = ({
  leftIcon,
  rightIcon,
  inputClassName,
  className,
  ...rest
}) => {
  return (
    <div
      className={cn(
        'flex items-center gap-2.5 rounded-full bg-surface-3 hover:bg-surface-highlight ' +
          'px-3.5 py-2 text-body text-text-main border border-transparent ' +
          'transition-colors duration-150 ease-out ' +
          'focus-within:bg-surface-3 focus-within:border-text-main/50 ' +
          'motion-reduce:transition-none',
        className
      )}
    >
      {leftIcon}
      <input
        className={cn(
          'w-full bg-transparent outline-none placeholder:text-text-subtle text-body',
          inputClassName
        )}
        {...rest}
      />
      {rightIcon}
    </div>
  );
};
