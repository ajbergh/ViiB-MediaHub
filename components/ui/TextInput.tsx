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
        'flex items-center gap-2 rounded-lg bg-surface-1 ring-1 ring-surface-3/80 ' +
          'px-3 py-2 text-body text-text-main ' +
          'transition-all duration-150 ease-out ' +
          'focus-within:ring-2 focus-within:ring-brand/40 focus-within:ring-offset-2 focus-within:ring-offset-surface-0 ' +
          'motion-reduce:transition-none',
        className
      )}
    >
      {leftIcon}
      <input
        className={cn(
          'w-full bg-transparent outline-none placeholder:text-text-subtle/90 text-body',
          inputClassName
        )}
        {...rest}
      />
      {rightIcon}
    </div>
  );
};
