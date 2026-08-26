/**
 * Native window behaviour for the compact player.
 *
 * The web build deliberately remains a layout-only version of skinny mode.
 * Wails provides the native sizing and always-on-top controls when available.
 */

type DesktopWindowSnapshot = {
  width: number;
  height: number;
  wasMaximised: boolean;
};

const DEFAULT_WINDOW_SIZE = { width: 1280, height: 820 };
// The desktop app uses a custom frameless title bar in regular mode. Skinny
// mode omits that bar, so its native window can match the player height.
const SKINNY_WINDOW_SIZE = { width: 920, height: 108 };
const SKINNY_MIN_WIDTH = 560;
const SKINNY_MAX_WIDTH = 1600;

let desktopWindowSnapshot: DesktopWindowSnapshot | null = null;
let syncRequestId = 0;

/** True when the app is running inside the Wails desktop shell. */
export const isNativeWindowRuntimeAvailable = (): boolean =>
  typeof window !== 'undefined' && 'runtime' in window;

export async function minimiseNativeWindow(): Promise<void> {
  if (!isNativeWindowRuntimeAvailable()) return;
  try {
    const runtime = await import('../backend/cmd/wails/frontend/wailsjs/runtime/runtime');
    runtime.WindowMinimise();
  } catch (error) {
    console.warn('[Window] Unable to minimise the native window', error);
  }
}

export async function toggleNativeWindowMaximise(): Promise<void> {
  if (!isNativeWindowRuntimeAvailable()) return;
  try {
    const runtime = await import('../backend/cmd/wails/frontend/wailsjs/runtime/runtime');
    runtime.WindowToggleMaximise();
  } catch (error) {
    console.warn('[Window] Unable to toggle native window maximisation', error);
  }
}

/** Mirrors the app's existing close-to-tray behaviour for the custom title bar. */
export async function hideNativeWindow(): Promise<void> {
  if (!isNativeWindowRuntimeAvailable()) return;
  try {
    const runtime = await import('../backend/cmd/wails/frontend/wailsjs/runtime/runtime');
    runtime.WindowHide();
  } catch (error) {
    console.warn('[Window] Unable to hide the native window', error);
  }
}

/**
 * Aligns the native Wails window with skinny mode state.
 * Browser builds still get the compact UI, but cannot alter their host window.
 */
export async function syncSkinnyWindow(
  isSkinnyMode: boolean,
  isAlwaysOnTop: boolean,
): Promise<void> {
  const requestId = ++syncRequestId;
  if (!isNativeWindowRuntimeAvailable()) return;

  try {
    const runtime = await import('../backend/cmd/wails/frontend/wailsjs/runtime/runtime');
    if (requestId !== syncRequestId) return;

    if (!isSkinnyMode) {
      runtime.WindowSetAlwaysOnTop(false);

      if (!desktopWindowSnapshot) return;

      const snapshot = desktopWindowSnapshot;
      desktopWindowSnapshot = null;
      runtime.WindowSetMinSize(900, 600);
      runtime.WindowSetMaxSize(0, 0);

      if (snapshot.wasMaximised) {
        runtime.WindowMaximise();
      } else {
        runtime.WindowSetSize(
          snapshot.width || DEFAULT_WINDOW_SIZE.width,
          snapshot.height || DEFAULT_WINDOW_SIZE.height,
        );
      }
      return;
    }

    if (!desktopWindowSnapshot) {
      const [size, wasMaximised] = await Promise.all([
        runtime.WindowGetSize(),
        runtime.WindowIsMaximised(),
      ]);
      if (requestId !== syncRequestId) return;

      desktopWindowSnapshot = {
        width: size.w,
        height: size.h,
        wasMaximised,
      };

      if (wasMaximised) runtime.WindowUnmaximise();
      runtime.WindowSetMinSize(SKINNY_MIN_WIDTH, SKINNY_WINDOW_SIZE.height);
      runtime.WindowSetMaxSize(SKINNY_MAX_WIDTH, SKINNY_WINDOW_SIZE.height);
      runtime.WindowSetSize(SKINNY_WINDOW_SIZE.width, SKINNY_WINDOW_SIZE.height);
    }

    runtime.WindowSetAlwaysOnTop(isAlwaysOnTop);
  } catch (error) {
    // Keep the compact web UI usable if a desktop runtime call is unavailable.
    console.warn('[SkinnyWindow] Native window update failed', error);
  }
}
