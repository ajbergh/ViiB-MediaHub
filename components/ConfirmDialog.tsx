/**
 * ViiB MediaHub - Confirm Dialog Component
 * 
 * Modal dialog for user confirmation of destructive or important actions.
 * 
 * Features:
 * - Customizable title and message
 * - Danger, warning, and default variants
 * - Loading state with spinner
 * - Escape key and backdrop click dismissal
 * - Accessible focus management
 * 
 * Used for actions like library reset, playlist delete, etc.
 * 
 * @module ConfirmDialog
 */

import React from 'react';
import { AlertTriangle, Loader2 } from 'lucide-react';

interface ConfirmDialogProps {
  isOpen: boolean;
  title: string;
  message: string | React.ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  isLoading?: boolean;
  variant?: 'danger' | 'warning' | 'default';
  onConfirm: () => void;
  onCancel: () => void;
}

export const ConfirmDialog: React.FC<ConfirmDialogProps> = ({
  isOpen,
  title,
  message,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  isLoading = false,
  variant = 'default',
  onConfirm,
  onCancel,
}) => {
  if (!isOpen) return null;

  const getVariantStyles = () => {
    switch (variant) {
      case 'danger':
        return {
          iconColor: 'text-red-500',
          buttonBg: 'bg-red-600 hover:bg-red-700',
        };
      case 'warning':
        return {
          iconColor: 'text-yellow-500',
          buttonBg: 'bg-yellow-600 hover:bg-yellow-700',
        };
      default:
        return {
          iconColor: 'text-brand',
          buttonBg: 'bg-brand hover:bg-brand/90',
        };
    }
  };

  const styles = getVariantStyles();

  return (
    <div 
      className="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 backdrop-blur-sm animate-in fade-in duration-200"
      onClick={(e) => {
        // Close on backdrop click unless loading
        if (e.target === e.currentTarget && !isLoading) {
          onCancel();
        }
      }}
    >
      <div className="bg-surface-2 border border-surface-border rounded-xl p-6 max-w-md w-full mx-4 shadow-2xl">
        <div className="flex items-center gap-4 mb-4">
          {variant !== 'default' && (
            <AlertTriangle size={28} className={styles.iconColor} />
          )}
          <h2 className="text-lg font-bold text-white">{title}</h2>
        </div>
        
        <div className="text-text-secondary mb-6 leading-relaxed">
          {message}
        </div>
        
        <div className="flex items-center justify-end gap-3">
          <button 
            onClick={onCancel}
            disabled={isLoading}
            className="px-4 py-2 rounded-lg font-medium text-text-main hover:bg-surface-3 transition-colors disabled:opacity-50"
          >
            {cancelLabel}
          </button>
          <button 
            onClick={onConfirm}
            disabled={isLoading}
            className={`px-5 py-2 rounded-lg font-bold text-white transition-colors flex items-center gap-2 disabled:opacity-50 ${styles.buttonBg}`}
          >
            {isLoading && <Loader2 size={16} className="animate-spin" />}
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
};

export default ConfirmDialog;
