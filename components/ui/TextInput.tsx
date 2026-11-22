/** Renders the shared styled text input. */

import React, { useRef } from 'react';
import { X } from 'lucide-react';
import { cn } from './cn';

export interface TextInputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
  inputClassName?: string;
  onClear?: () => void;
  clearLabel?: string;
}

export const TextInput: React.FC<TextInputProps> = ({
  leftIcon,
  rightIcon,
  inputClassName,
  className,
  onClear,
  clearLabel = 'Clear input',
  value,
  ...rest
}) => {
  const inputRef = useRef<HTMLInputElement>(null);
  const showClear = Boolean(onClear && typeof value === 'string' && value.length > 0);

  return (
    <div
      onClick={(e) => {
        if (e.target === e.currentTarget) inputRef.current?.focus();
      }}
      className={cn(
        'flex items-center gap-2.5 rounded-full bg-surface-3 hover:bg-surface-highlight ' +
          'px-3.5 py-2 min-h-[40px] text-body text-text-main border border-transparent cursor-text ' +
          'transition-colors duration-150 ease-out ' +
          'focus-within:bg-surface-3 focus-within:border-text-main/50 ' +
          'motion-reduce:transition-none',
        className
      )}
    >
      {leftIcon}
      <input
        ref={inputRef}
        value={value}
        className={cn(
          'w-full min-w-0 bg-transparent outline-none placeholder:text-text-subtle text-body',
          inputClassName
        )}
        {...rest}
      />
      {showClear ? (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onClear?.();
            inputRef.current?.focus();
          }}
          aria-label={clearLabel}
          className="p-1 -mr-1 rounded-full text-text-secondary hover:text-text-main hover:bg-surface-hover transition-colors shrink-0 inline-flex items-center justify-center min-w-[28px] min-h-[28px]"
        >
          <X size={15} />
        </button>
      ) : null}
      {rightIcon}
    </div>
  );
};
