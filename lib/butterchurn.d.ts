/**
 * Type declarations for Butterchurn Milkdrop visualizer library
 * 
 * Butterchurn is a WebGL 2 implementation of the classic Winamp Milkdrop visualizer.
 * These types are based on the library's API as documented in the GitHub repo.
 * 
 * @see https://github.com/jberg/butterchurn
 */

declare module 'butterchurn' {
  export interface ButterchurnVisualizerOptions {
    /** Canvas width in pixels */
    width: number;
    /** Canvas height in pixels */
    height: number;
    /** Device pixel ratio for high-DPI displays */
    pixelRatio?: number;
    /** Texture ratio for internal rendering */
    textureRatio?: number;
    /** Mesh size for visualization (default: 48) */
    meshWidth?: number;
    meshHeight?: number;
  }

  export interface ButterchurnVisualizer {
    /**
     * Connect an audio node for visualization
     * @param audioNode The audio node to analyze (e.g., GainNode, AnalyserNode)
     */
    connectAudio(audioNode: AudioNode): void;

    /**
     * Load a preset for visualization
     * @param preset The preset object to load
     * @param blendTime Seconds to blend/transition to the new preset (0 = instant)
     */
    loadPreset(preset: object, blendTime: number): void;

    /**
     * Render a single frame of the visualization
     * Call this in a requestAnimationFrame loop
     */
    render(): void;

    /**
     * Update the renderer size (e.g., when canvas is resized)
     * @param width New width in pixels
     * @param height New height in pixels
     */
    setRendererSize(width: number, height: number): void;

    /**
     * Manually lose the WebGL context (for cleanup)
     */
    loseGLContext?(): void;

    /**
     * Display song title animation
     * @param title The song title to display
     */
    launchSongTitleAnim(title: string): void;
  }

  export interface Butterchurn {
    /**
     * Create a new Butterchurn visualizer instance
     * @param audioContext The Web Audio API AudioContext
     * @param canvas The HTMLCanvasElement to render to
     * @param options Configuration options
     */
    createVisualizer(
      audioContext: AudioContext,
      canvas: HTMLCanvasElement,
      options: ButterchurnVisualizerOptions
    ): ButterchurnVisualizer;
  }

  const butterchurn: Butterchurn;
  export default butterchurn;
}

declare module 'butterchurn-presets' {
  export interface ButterchurnPresets {
    /**
     * Get all available presets as a record of preset name to preset object
     * @returns Record of preset names to preset objects
     */
    getPresets(): Record<string, object>;

    /**
     * Get list of preset keys (names)
     * @returns Array of preset key strings
     */
    getPresetKeys?(): string[];
  }

  const butterchurnPresets: ButterchurnPresets;
  export default butterchurnPresets;
}

declare module 'butterchurn-presets/lib/butterchurnPresetsExtra.min' {
  export interface ButterchurnPresetsExtra {
    /**
     * Get all extra presets as a record of preset name to preset object
     * @returns Record of preset names to preset objects
     */
    getPresets(): Record<string, object>;
  }

  const butterchurnPresetsExtra: ButterchurnPresetsExtra;
  export default butterchurnPresetsExtra;
}
