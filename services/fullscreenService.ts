/**
 * ViiB MediaHub - Fullscreen Service
 * 
 * Provides unified fullscreen control that works in both:
 * - Wails mode: Uses native Wails runtime API for true borderless fullscreen
 * - Browser mode: Uses standard Fullscreen API as fallback
 * 
 * @module fullscreenService
 */

// Check if we're running in Wails (native app)
const isWailsEnvironment = (): boolean => {
    return typeof window !== 'undefined' && 'runtime' in window;
};

/**
 * Enter fullscreen mode
 * - In Wails: Uses native window fullscreen
 * - In browser: Uses standard Fullscreen API
 */
export async function enterFullscreen(): Promise<void> {
    if (isWailsEnvironment()) {
        try {
            // Dynamic import to avoid bundling issues when not in Wails
            const { WindowFullscreen } = await import('../backend/cmd/wails/frontend/wailsjs/runtime/runtime');
            WindowFullscreen();
            console.log('🖥️ Wails fullscreen enabled');
        } catch (e) {
            console.warn('Wails fullscreen failed, falling back to browser API', e);
            await enterBrowserFullscreen();
        }
    } else {
        await enterBrowserFullscreen();
    }
}

/**
 * Exit fullscreen mode
 * - In Wails: Restores previous window dimensions
 * - In browser: Uses standard Fullscreen API
 */
export async function exitFullscreen(): Promise<void> {
    if (isWailsEnvironment()) {
        try {
            const { WindowUnfullscreen } = await import('../backend/cmd/wails/frontend/wailsjs/runtime/runtime');
            WindowUnfullscreen();
            console.log('🖥️ Wails fullscreen disabled');
        } catch (e) {
            console.warn('Wails unfullscreen failed, falling back to browser API', e);
            exitBrowserFullscreen();
        }
    } else {
        exitBrowserFullscreen();
    }
}

/**
 * Check if currently in fullscreen mode
 */
export async function isFullscreen(): Promise<boolean> {
    if (isWailsEnvironment()) {
        try {
            const { WindowIsFullscreen } = await import('../backend/cmd/wails/frontend/wailsjs/runtime/runtime');
            return await WindowIsFullscreen();
        } catch (e) {
            console.warn('Wails isFullscreen failed, checking browser state', e);
            return isBrowserFullscreen();
        }
    }
    return isBrowserFullscreen();
}

/**
 * Toggle fullscreen mode
 */
export async function toggleFullscreen(): Promise<boolean> {
    const current = await isFullscreen();
    if (current) {
        await exitFullscreen();
        return false;
    } else {
        await enterFullscreen();
        return true;
    }
}

// Browser Fullscreen API helpers
async function enterBrowserFullscreen(): Promise<void> {
    const elem = document.documentElement;
    try {
        if (elem.requestFullscreen) {
            await elem.requestFullscreen();
        } else if ((elem as any).webkitRequestFullscreen) {
            await (elem as any).webkitRequestFullscreen();
        } else if ((elem as any).msRequestFullscreen) {
            await (elem as any).msRequestFullscreen();
        }
        console.log('🖥️ Browser fullscreen enabled');
    } catch (e) {
        console.warn('Browser fullscreen failed', e);
    }
}

function exitBrowserFullscreen(): void {
    try {
        if (document.exitFullscreen) {
            document.exitFullscreen();
        } else if ((document as any).webkitExitFullscreen) {
            (document as any).webkitExitFullscreen();
        } else if ((document as any).msExitFullscreen) {
            (document as any).msExitFullscreen();
        }
        console.log('🖥️ Browser fullscreen disabled');
    } catch (e) {
        console.warn('Browser exit fullscreen failed', e);
    }
}

function isBrowserFullscreen(): boolean {
    return !!(
        document.fullscreenElement ||
        (document as any).webkitFullscreenElement ||
        (document as any).msFullscreenElement
    );
}
