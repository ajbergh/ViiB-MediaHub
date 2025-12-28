/**
 * ViiB MediaHub - Toast Notification System
 * 
 * Provides user-visible notifications for success, error, info, and warning messages.
 * 
 * Features:
 * - Auto-dismiss with configurable duration
 * - Multiple toast types (success, error, info, warning)
 * - Action buttons for undo/retry operations
 * - Stacked toasts with animation
 * - Accessible with proper ARIA roles
 * 
 * Usage:
 * const { showToast } = useStore();
 * showToast({ type: 'success', message: 'Song added to queue' });
 * showToast({ type: 'error', message: 'Failed to load', action: { label: 'Retry', onClick: retry } });
 * 
 * @module Toast
 */

import React, { useEffect, useState } from 'react';
import { X, CheckCircle, AlertCircle, Info, AlertTriangle } from 'lucide-react';
import { useStore } from '../store';

export interface Toast {
  id: string;
  type: 'success' | 'error' | 'info' | 'warning';
  message: string;
  duration?: number;
  action?: {
    label: string;
    onClick: () => void;
  };
}

const iconMap = {
  success: CheckCircle,
  error: AlertCircle,
  info: Info,
  warning: AlertTriangle,
};

const colorMap = {
  success: 'bg-brand text-surface-0',
  error: 'bg-error text-surface-0',
  info: 'bg-accent-blue text-surface-0',
  warning: 'bg-warning text-surface-0',
};

const bgColorMap = {
  success: 'bg-surface-2 border-brand/30',
  error: 'bg-surface-2 border-error/30',
  info: 'bg-surface-2 border-accent-blue/30',
  warning: 'bg-surface-2 border-warning/30',
};

interface ToastItemProps {
  toast: Toast;
  onDismiss: (id: string) => void;
}

const ToastItem: React.FC<ToastItemProps> = ({ toast, onDismiss }) => {
  const [isExiting, setIsExiting] = useState(false);
  const Icon = iconMap[toast.type];
  
  useEffect(() => {
    const duration = toast.duration ?? 4000;
    if (duration > 0) {
      const timer = setTimeout(() => {
        setIsExiting(true);
        setTimeout(() => onDismiss(toast.id), 200);
      }, duration);
      return () => clearTimeout(timer);
    }
  }, [toast, onDismiss]);

  const handleDismiss = () => {
    setIsExiting(true);
    setTimeout(() => onDismiss(toast.id), 200);
  };

  return (
    <div
      role="alert"
      aria-live="polite"
      className={`
        flex items-center gap-3 px-4 py-3 rounded-lg border shadow-xl backdrop-blur-sm
        ${bgColorMap[toast.type]}
        ${isExiting ? 'animate-out fade-out slide-out-to-right-full duration-200' : 'animate-in fade-in slide-in-from-right-full duration-300'}
        motion-reduce:animate-none motion-reduce:transition-none
      `}
    >
      <div className={`p-1 rounded-full ${colorMap[toast.type]}`}>
        <Icon size={16} />
      </div>
      
      <span className="flex-1 text-sm text-text-main font-medium">
        {toast.message}
      </span>
      
      {toast.action && (
        <button
          onClick={() => {
            toast.action?.onClick();
            handleDismiss();
          }}
          className="text-sm font-bold text-brand hover:text-brand-hover transition-colors px-2 py-1 rounded hover:bg-white/5"
        >
          {toast.action.label}
        </button>
      )}
      
      <button
        onClick={handleDismiss}
        className="p-1 text-text-subtle hover:text-text-main transition-colors rounded-full hover:bg-white/10"
        aria-label="Dismiss notification"
      >
        <X size={16} />
      </button>
    </div>
  );
};

/**
 * Toast container - renders all active toasts
 * Place this once in your app layout
 */
export const ToastContainer: React.FC = () => {
  const { toasts, dismissToast } = useStore();

  if (toasts.length === 0) return null;

  return (
    <div 
      className="fixed bottom-28 right-4 z-[200] flex flex-col gap-2 max-w-sm w-full pointer-events-none"
      aria-label="Notifications"
    >
      {toasts.map((toast) => (
        <div key={toast.id} className="pointer-events-auto">
          <ToastItem toast={toast} onDismiss={dismissToast} />
        </div>
      ))}
    </div>
  );
};
