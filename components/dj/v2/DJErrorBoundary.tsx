/**
 * ViiB MediaHub - DJ Mode Error Boundary
 * 
 * Custom error boundary for DJ Mode components that provides:
 * - Persistent logging of errors to viib.log
 * - User-friendly error display with retry option
 * - Component-level error isolation
 * 
 * @module components/dj/v2/DJErrorBoundary
 */

import React, { Component, ReactNode } from 'react';
import { createLogger, flushPendingLogs } from '../../../services/loggerService';

const logger = createLogger('DJErrorBoundary');

interface DJErrorBoundaryProps {
  children: ReactNode;
  /** Component name for logging context */
  componentName?: string;
  /** Fallback UI to show on error (defaults to retry button) */
  fallback?: ReactNode;
  /** Called when error occurs */
  onError?: (error: Error, errorInfo: React.ErrorInfo) => void;
}

interface DJErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
  errorInfo: React.ErrorInfo | null;
}

export class DJErrorBoundary extends Component<DJErrorBoundaryProps, DJErrorBoundaryState> {
  constructor(props: DJErrorBoundaryProps) {
    super(props);
    this.state = { hasError: false, error: null, errorInfo: null };
  }

  static getDerivedStateFromError(error: Error): Partial<DJErrorBoundaryState> {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    const componentName = this.props.componentName || 'Unknown';
    
    // Log to persistent storage
    logger.error(`Crash in ${componentName}`, {
      error: {
        name: error.name,
        message: error.message,
        stack: error.stack,
      },
      componentStack: errorInfo.componentStack,
    });
    
    // Ensure logs are flushed immediately
    flushPendingLogs();
    
    this.setState({ errorInfo });
    
    // Call custom handler if provided
    this.props.onError?.(error, errorInfo);
  }

  handleRetry = () => {
    logger.info('User requested retry after crash');
    this.setState({ hasError: false, error: null, errorInfo: null });
  };

  render() {
    if (this.state.hasError) {
      // Custom fallback if provided
      if (this.props.fallback) {
        return this.props.fallback;
      }
      
      // Default fallback UI
      const componentName = this.props.componentName || 'Component';
      
      return (
        <div className="flex flex-col items-center justify-center p-4 bg-red-900/20 border border-red-500/30 rounded-lg">
          <div className="text-red-400 text-sm font-medium mb-2">
            {componentName} encountered an error
          </div>
          <div className="text-neutral-400 text-xs mb-3 max-w-xs text-center">
            {this.state.error?.message || 'Unknown error'}
          </div>
          <button
            onClick={this.handleRetry}
            className="px-3 py-1.5 bg-red-600 hover:bg-red-500 text-white text-xs font-medium rounded transition-colors"
          >
            Retry
          </button>
        </div>
      );
    }

    return this.props.children;
  }
}

/**
 * HOC to wrap a component with DJErrorBoundary
 */
export function withDJErrorBoundary<P extends object>(
  WrappedComponent: React.ComponentType<P>,
  componentName: string
) {
  const WithErrorBoundary: React.FC<P> = (props) => (
    <DJErrorBoundary componentName={componentName}>
      <WrappedComponent {...props} />
    </DJErrorBoundary>
  );
  
  WithErrorBoundary.displayName = `withDJErrorBoundary(${componentName})`;
  return WithErrorBoundary;
}

export default DJErrorBoundary;
