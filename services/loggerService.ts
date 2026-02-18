/**
 * ViiB MediaHub - Frontend Logger Service
 * 
 * Sends log messages to the backend API for persistent storage in viib.log.
 * Also logs to console for dev tools visibility.
 * 
 * @module services/loggerService
 */

// API base URL - matches the api.ts configuration
const API_BASE = '/api';

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

interface LogEntry {
  level: LogLevel;
  message: string;
  component: string;
  data?: unknown;
}

// Buffer for batching log entries (reduce network requests)
let logBuffer: LogEntry[] = [];
let flushTimeout: ReturnType<typeof setTimeout> | null = null;
const FLUSH_INTERVAL = 500; // ms - batch logs every 500ms
const MAX_BUFFER_SIZE = 20; // Flush immediately if buffer gets this big

/**
 * Send buffered logs to the backend
 */
async function flushLogs(): Promise<void> {
  if (logBuffer.length === 0) return;
  
  const entries = [...logBuffer];
  logBuffer = [];
  
  // Send each log entry (could batch in future API version)
  for (const entry of entries) {
    try {
      await fetch(`${API_BASE}/log`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(entry),
      });
    } catch (err) {
      // Silently fail - don't want logging to break the app
      // Fall back to console only
      console.warn('[Logger] Failed to send log to backend:', err);
    }
  }
}

/**
 * Schedule a flush of the log buffer
 */
function scheduleFlush(): void {
  if (flushTimeout) return;
  
  flushTimeout = setTimeout(() => {
    flushTimeout = null;
    flushLogs();
  }, FLUSH_INTERVAL);
}

/**
 * Add a log entry to the buffer
 */
function addToBuffer(entry: LogEntry): void {
  logBuffer.push(entry);
  
  // Flush immediately for errors or if buffer is full
  if (entry.level === 'error' || logBuffer.length >= MAX_BUFFER_SIZE) {
    if (flushTimeout) {
      clearTimeout(flushTimeout);
      flushTimeout = null;
    }
    flushLogs();
  } else {
    scheduleFlush();
  }
}

/**
 * Format data for console output
 */
function formatData(data: unknown): string {
  if (data === undefined || data === null) return '';
  try {
    return JSON.stringify(data, null, 2);
  } catch {
    return String(data);
  }
}

/**
 * Create a logger instance for a specific component
 */
export function createLogger(component: string) {
  return {
    debug(message: string, data?: unknown): void {
      const entry: LogEntry = { level: 'debug', message, component, data };
      console.debug(`[${component}]`, message, data !== undefined ? data : '');
      addToBuffer(entry);
    },
    
    info(message: string, data?: unknown): void {
      const entry: LogEntry = { level: 'info', message, component, data };
      console.info(`[${component}]`, message, data !== undefined ? data : '');
      addToBuffer(entry);
    },
    
    warn(message: string, data?: unknown): void {
      const entry: LogEntry = { level: 'warn', message, component, data };
      console.warn(`[${component}]`, message, data !== undefined ? data : '');
      addToBuffer(entry);
    },
    
    error(message: string, data?: unknown): void {
      const entry: LogEntry = { level: 'error', message, component, data };
      console.error(`[${component}]`, message, data !== undefined ? data : '');
      addToBuffer(entry);
    },
    
    /**
     * Log an error object with stack trace
     */
    logError(error: Error | unknown, context?: string): void {
      const errorObj = error instanceof Error ? error : new Error(String(error));
      const message = context 
        ? `${context}: ${errorObj.message}` 
        : errorObj.message;
      
      const entry: LogEntry = { 
        level: 'error', 
        message, 
        component, 
        data: {
          name: errorObj.name,
          message: errorObj.message,
          stack: errorObj.stack,
        }
      };
      
      console.error(`[${component}]`, message, errorObj);
      addToBuffer(entry);
    }
  };
}

/**
 * Global logger for non-component-specific logging
 */
export const logger = createLogger('App');

/**
 * Flush any pending logs (call before app unmounts or on errors)
 */
export function flushPendingLogs(): Promise<void> {
  if (flushTimeout) {
    clearTimeout(flushTimeout);
    flushTimeout = null;
  }
  return flushLogs();
}

/**
 * Log unhandled errors and promise rejections
 */
export function setupGlobalErrorHandlers(): void {
  const errorLogger = createLogger('GlobalError');
  
  // Handle uncaught errors
  window.addEventListener('error', (event) => {
    errorLogger.logError(event.error || event.message, 'Uncaught error');
  });
  
  // Handle unhandled promise rejections
  window.addEventListener('unhandledrejection', (event) => {
    errorLogger.logError(event.reason, 'Unhandled promise rejection');
  });
  
  // Flush logs before page unload
  window.addEventListener('beforeunload', () => {
    flushPendingLogs();
  });
}

export default logger;
