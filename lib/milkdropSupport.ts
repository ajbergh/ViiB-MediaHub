/**
 * ViiB MediaHub - Milkdrop Support Detection
 * 
 * Utility functions to check browser compatibility for Milkdrop visualizations
 * via the Butterchurn WebGL library.
 * 
 * Requirements:
 * - WebGL 2 support (required for Butterchurn rendering)
 * - Web Audio API (already used by ViiB audio engine)
 * 
 * Browser Support:
 * - Chrome 56+ ✅
 * - Firefox 51+ ✅
 * - Safari 15+ ✅
 * - Edge 79+ ✅
 * 
 * @module milkdropSupport
 */


/**
 * Check if the browser supports Milkdrop visualizations.
 * Requires WebGL 2 and Web Audio API.
 * 
 * @returns {boolean} True if Milkdrop is supported
 */
export function isMilkdropSupported(): boolean {
  // Check WebGL 2
  const canvas = document.createElement('canvas');
  let gl: WebGL2RenderingContext | null = null;
  
  try {
    gl = canvas.getContext('webgl2');
  } catch (e) {
    gl = null;
  }
  
  if (!gl) {
    console.warn('[Milkdrop] WebGL 2 not supported');
    return false;
  }
  
  // Check Web Audio API
  const audioApiSupported = !!(window.AudioContext || (window as any).webkitAudioContext);
  
  if (!audioApiSupported) {
    console.warn('[Milkdrop] Web Audio API not supported');
    return false;
  }
  
  return true;
}

/**
 * Get recommended quality setting based on device capabilities.
 * Considers mobile devices, memory constraints, and GPU type.
 * 
 * @returns {'low' | 'medium' | 'high'} Recommended quality setting
 */
export function getRecommendedQuality(): 'low' | 'medium' | 'high' {
  // Check for mobile/low-power devices
  const isMobile = /Android|webOS|iPhone|iPad|iPod/i.test(navigator.userAgent);
  const hasLowMemory = (navigator as any).deviceMemory && (navigator as any).deviceMemory < 4;
  
  if (isMobile || hasLowMemory) {
    return 'low';
  }
  
  // Check GPU capabilities (heuristic)
  const canvas = document.createElement('canvas');
  const gl = canvas.getContext('webgl2');
  
  if (gl) {
    const debugInfo = gl.getExtension('WEBGL_debug_renderer_info');
    if (debugInfo) {
      const renderer = gl.getParameter(debugInfo.UNMASKED_RENDERER_WEBGL);
      // Intel integrated graphics → medium quality
      if (/Intel/i.test(renderer)) {
        return 'medium';
      }
    }
  }
  
  return 'high';
}

/**
 * Quality resolution mapping for Milkdrop rendering.
 * Higher quality = larger canvas resolution = more GPU load.
 */
export const MILKDROP_QUALITY_MAP = {
  low: { width: 640, height: 480 },
  medium: { width: 1280, height: 720 },
  high: { width: 1920, height: 1080 }
} as const;

/**
 * Get WebGL 2 capabilities for debugging/info purposes.
 * 
 * @returns {object | null} WebGL capabilities or null if not supported
 */
export function getWebGLCapabilities(): { 
  renderer: string; 
  vendor: string; 
  maxTextureSize: number;
  maxViewportDims: number[];
} | null {
  const canvas = document.createElement('canvas');
  const gl = canvas.getContext('webgl2');
  
  if (!gl) return null;
  
  const debugInfo = gl.getExtension('WEBGL_debug_renderer_info');
  
  return {
    renderer: debugInfo 
      ? gl.getParameter(debugInfo.UNMASKED_RENDERER_WEBGL) 
      : 'Unknown',
    vendor: debugInfo 
      ? gl.getParameter(debugInfo.UNMASKED_VENDOR_WEBGL) 
      : 'Unknown',
    maxTextureSize: gl.getParameter(gl.MAX_TEXTURE_SIZE),
    maxViewportDims: gl.getParameter(gl.MAX_VIEWPORT_DIMS) as number[]
  };
}
