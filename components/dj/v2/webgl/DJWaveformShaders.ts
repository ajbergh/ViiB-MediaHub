/**
 * DJ Waveform Shaders
 * 
 * WebGL2 shaders for high-performance DJ waveform rendering.
 * Includes main waveform, beat grid, hot cue markers, and playhead.
 * 
 * @module DJWaveformShaders
 */

/**
 * Fullscreen quad vertices for fragment shader rendering
 */
export const FULLSCREEN_QUAD_VERTICES = new Float32Array([
  -1, -1,
   1, -1,
  -1,  1,
  -1,  1,
   1, -1,
   1,  1
]);

/**
 * Common vertex shader for DJ waveforms
 */
export const djWaveformVertexShader = `#version 300 es
precision highp float;

in vec2 a_position;
out vec2 v_uv;

void main() {
    v_uv = a_position * 0.5 + 0.5;
    gl_Position = vec4(a_position, 0.0, 1.0);
}
`;

/**
 * WebGL1 fallback vertex shader
 */
export const djWaveformVertexShaderWebGL1 = `
precision highp float;

attribute vec2 a_position;
varying vec2 v_uv;

void main() {
    v_uv = a_position * 0.5 + 0.5;
    gl_Position = vec4(a_position, 0.0, 1.0);
}
`;

/** Converts the shared WebGL 2 fragment sources to WebGL 1 GLSL syntax. */
export function toDJWebGL1FragmentShader(source: string): string {
  return source
    .replace(/^\s*#version\s+300\s+es\s*\n/m, '')
    .replace(/^\s*in\s+vec2\s+v_uv\s*;\s*$/m, 'varying vec2 v_uv;')
    .replace(/^\s*out\s+vec4\s+fragColor\s*;\s*$/m, '')
    .replace(/\bfragColor\b/g, 'gl_FragColor')
    .replace(/\btexture\s*\(/g, 'texture2D(');
}

/**
 * Main waveform fragment shader with amplitude palettes
 * 
 * Features:
 * - Gradient, amplitude-level and deck-color palettes
 * - Smooth scrolling via uniform offset
 * - Center-line symmetric rendering
 * - Anti-aliased edges
 */
export const djWaveformFragmentShader = `#version 300 es
precision highp float;

uniform sampler2D u_waveformTex;
uniform float u_position;        // Current playback position (0-1 normalized)
uniform float u_visibleRange;    // Visible time range / total duration
uniform vec2 u_resolution;
uniform float u_centerY;         // Center line Y position (0-1)
uniform vec3 u_deckColor;        // Deck accent color
uniform int u_hasPeaks;          // 1 if waveform data available
uniform float u_peakCount;
uniform float u_peakWidth;
uniform float u_peakHeight;
uniform float u_loopStart;
uniform float u_loopEnd;
uniform int u_loopEnabled;
uniform int u_colorMode;

// Vertical gradient palette. This waveform stores amplitude only, not frequency bands.
const vec3 BASS_COLOR = vec3(1.0, 0.267, 0.267);    // #ff4444
const vec3 LOW_MID_COLOR = vec3(1.0, 0.533, 0.267); // #ff8844
const vec3 MID_COLOR = vec3(0.267, 1.0, 0.267);     // #44ff44
const vec3 HIGH_MID_COLOR = vec3(0.267, 1.0, 1.0);  // #44ffff
const vec3 HIGH_COLOR = vec3(0.267, 0.267, 1.0);    // #4444ff
const vec3 BG_COLOR = vec3(0.071, 0.071, 0.071);    // #121212

in vec2 v_uv;
out vec4 fragColor;

vec3 getGradientColor(float normalizedY) {
    float band = normalizedY;
    
    if (band < 0.2) {
        return mix(BASS_COLOR, LOW_MID_COLOR, band / 0.2);
    } else if (band < 0.4) {
        return mix(LOW_MID_COLOR, MID_COLOR, (band - 0.2) / 0.2);
    } else if (band < 0.6) {
        return MID_COLOR;
    } else if (band < 0.8) {
        return mix(MID_COLOR, HIGH_MID_COLOR, (band - 0.6) / 0.2);
    } else {
        return mix(HIGH_MID_COLOR, HIGH_COLOR, (band - 0.8) / 0.2);
    }
}

void main() {
    // Calculate sample position (scrolling waveform)
    float playheadU = 0.5; // Playhead at center
    float sampleU = u_position + (v_uv.x - playheadU) * u_visibleRange;
    
    // Out of bounds check
    if (sampleU < 0.0 || sampleU > 1.0 || u_hasPeaks == 0) {
        // Show placeholder or background
        if (u_hasPeaks == 0) {
            // No waveform data - show dotted center line
            float dotPattern = mod(v_uv.x * u_resolution.x, 8.0);
            if (abs(v_uv.y - u_centerY) < 0.005 && dotPattern < 4.0) {
                fragColor = vec4(0.3, 0.3, 0.3, 1.0);
                return;
            }
        }
        fragColor = vec4(BG_COLOR, 1.0);
        return;
    }
    
    // Sample waveform peak from texture
    float samplesPerPixel = max(1.0, u_visibleRange * u_peakCount / u_resolution.x);
    float sampleCount = min(64.0, ceil(samplesPerPixel));
    float centerIndex = sampleU * u_peakCount;
    float firstIndex = centerIndex - samplesPerPixel * 0.5;
    float peak = 0.0;
    for (int i = 0; i < 64; i++) {
        if (float(i) >= sampleCount) break;
        float sampleIndex = clamp(firstIndex + (float(i) + 0.5) * samplesPerPixel / sampleCount, 0.0, u_peakCount - 1.0);
        float texX = (mod(floor(sampleIndex), u_peakWidth) + 0.5) / u_peakWidth;
        float texY = (floor(sampleIndex / u_peakWidth) + 0.5) / u_peakHeight;
        peak = max(peak, texture(u_waveformTex, vec2(texX, texY)).r);
    }
    bool hasLoop = u_loopStart >= 0.0 && u_loopEnd > u_loopStart;
    float loopStartPixels = abs((sampleU - u_loopStart) / max(u_visibleRange, 0.000001) * u_resolution.x);
    float loopEndPixels = abs((sampleU - u_loopEnd) / max(u_visibleRange, 0.000001) * u_resolution.x);
    if (hasLoop && min(loopStartPixels, loopEndPixels) < 1.25) {
        fragColor = vec4(u_loopEnabled == 1 ? vec3(0.2, 1.0, 0.55) : vec3(0.6, 0.7, 0.8), 0.95);
        return;
    }
    
    // Distance from center line
    float distFromCenter = abs(v_uv.y - u_centerY) * 2.0;
    
    // Anti-aliasing: smooth edge transition
    float edgeWidth = 1.5 / u_resolution.y;
    float alpha = 1.0 - smoothstep(peak - edgeWidth, peak + edgeWidth, distFromCenter);
    
    // Within waveform amplitude?
    if (alpha > 0.01) {
        // Gradient mode varies color with distance from the center line.
        float freqPosition = distFromCenter / max(peak, 0.001);
        vec3 color = getGradientColor(freqPosition);
        if (u_colorMode == 1) {
            color = peak > 0.66 ? vec3(1.0, 0.27, 0.27) : (peak > 0.33 ? vec3(1.0, 0.67, 0.2) : vec3(0.27, 0.55, 1.0));
        } else if (u_colorMode == 2) {
            color = u_deckColor;
        }
        
        // Slight brightness variation for depth
        color *= 0.85 + 0.15 * (1.0 - freqPosition);
        
        bool inLoop = hasLoop && sampleU >= u_loopStart && sampleU <= u_loopEnd;
        if (inLoop) color = mix(color, u_loopEnabled == 1 ? vec3(0.12, 0.82, 0.42) : vec3(0.55, 0.65, 0.72), u_loopEnabled == 1 ? 0.42 : 0.18);
        fragColor = vec4(color, alpha);
    } else {
        // Background
        bool inLoop = hasLoop && sampleU >= u_loopStart && sampleU <= u_loopEnd;
        vec3 background = inLoop ? mix(BG_COLOR, u_loopEnabled == 1 ? vec3(0.12, 0.82, 0.42) : vec3(0.55, 0.65, 0.72), u_loopEnabled == 1 ? 0.2 : 0.08) : BG_COLOR;
        fragColor = vec4(background, 1.0);
    }
}
`;

/**
 * Beat grid overlay fragment shader
 * 
 * Features:
 * - Downbeat emphasis (every 4 beats)
 * - Smooth line rendering
 * - BPM-synchronized positioning
 */
export const djBeatGridFragmentShader = `#version 300 es
precision highp float;

uniform float u_position;        // Current position in seconds
uniform float u_bpm;             // Beats per minute
uniform float u_visibleSeconds;  // Visible time window
uniform float u_beatOffset;      // First beat offset in seconds
uniform vec2 u_resolution;

in vec2 v_uv;
out vec4 fragColor;

void main() {
    // Calculate time at this pixel
    float pixelTime = u_position + (v_uv.x - 0.5) * u_visibleSeconds;
    
    if (pixelTime < 0.0 || u_bpm <= 0.0) {
        discard;
    }
    
    // Beat timing
    float secondsPerBeat = 60.0 / u_bpm;
    float beatTime = pixelTime - u_beatOffset;
    float beatIndex = floor(beatTime / secondsPerBeat);
    float beatPhase = mod(beatTime, secondsPerBeat);
    
    // Distance to nearest beat line (in time)
    float distToBeat = min(beatPhase, secondsPerBeat - beatPhase);
    
    // Convert to pixels for line width
    float pixelsPerSecond = u_resolution.x / u_visibleSeconds;
    float distInPixels = distToBeat * pixelsPerSecond;
    
    // Determine line properties
    bool isDownbeat = mod(beatIndex, 4.0) < 0.5;
    float lineWidth = isDownbeat ? 1.5 : 0.75;
    float alpha = isDownbeat ? 0.4 : 0.15;
    
    // Smooth line rendering
    if (distInPixels < lineWidth) {
        float edgeSoftness = 1.0 - smoothstep(0.0, lineWidth, distInPixels);
        fragColor = vec4(1.0, 1.0, 1.0, alpha * edgeSoftness);
    } else {
        discard;
    }
}
`;

/**
 * Playhead fragment shader
 * 
 * Renders a vertical playhead line with triangle marker
 */
export const djPlayheadFragmentShader = `#version 300 es
precision highp float;

uniform vec2 u_resolution;
uniform vec3 u_color;            // Playhead color (default: red)
uniform float u_markerSize;      // Size of triangle marker

in vec2 v_uv;
out vec4 fragColor;

void main() {
    vec2 pixel = v_uv * u_resolution;
    float centerX = u_resolution.x * 0.5;
    
    // Main vertical line
    float distToLine = abs(pixel.x - centerX);
    if (distToLine < 1.0) {
        fragColor = vec4(u_color, 1.0);
        return;
    }
    
    // Triangle marker at top
    float triangleY = u_markerSize;
    if (pixel.y < triangleY) {
        float triangleWidth = u_markerSize * (1.0 - pixel.y / triangleY);
        if (distToLine < triangleWidth) {
            fragColor = vec4(u_color, 1.0);
            return;
        }
    }
    
    discard;
}
`;

/**
 * Hot cue marker fragment shader
 * 
 * Renders triangular hot cue markers at specific positions
 */
export const djHotCueFragmentShader = `#version 300 es
precision highp float;

uniform vec2 u_resolution;
uniform float u_position;        // Current playback position (0-1)
uniform float u_visibleRange;    // Visible time range / duration
uniform int u_cueCount;          // Number of hot cues (max 8)
uniform vec4 u_cuePositions[8];  // x=position(0-1), y=r, z=g, w=b
uniform float u_markerSize;      // Marker size in pixels

in vec2 v_uv;
out vec4 fragColor;

void main() {
    vec2 pixel = v_uv * u_resolution;
    
    for (int i = 0; i < 8; i++) {
        if (i >= u_cueCount) break;
        
        vec4 cue = u_cuePositions[i];
        float cuePos = cue.x;
        vec3 cueColor = cue.yzw;
        
        // Convert cue position to screen X
        float relativePos = (cuePos - u_position) / u_visibleRange + 0.5;
        if (relativePos < 0.0 || relativePos > 1.0) continue;
        
        float cueX = relativePos * u_resolution.x;
        float distToCue = abs(pixel.x - cueX);
        
        // Triangle marker
        if (distToCue < u_markerSize && pixel.y < u_markerSize) {
            float triangleWidth = u_markerSize * (1.0 - pixel.y / u_markerSize);
            if (distToCue < triangleWidth) {
                fragColor = vec4(cueColor, 1.0);
                return;
            }
        }
        
        // Vertical line (faint)
        if (distToCue < 1.0) {
            fragColor = vec4(cueColor, 0.3);
            return;
        }
    }
    
    discard;
}
`;

/**
 * Overview waveform fragment shader (mini strip)
 * 
 * Simplified waveform for the overview strip with playhead position
 */
export const djOverviewFragmentShader = `#version 300 es
precision highp float;

uniform sampler2D u_waveformTex;
uniform vec2 u_resolution;
uniform float u_position;        // Current playhead position (0-1)
uniform vec3 u_deckColor;        // Deck accent color
uniform int u_hasPeaks;          // 1 if waveform data available
uniform float u_peakCount;
uniform float u_peakWidth;
uniform float u_peakHeight;
uniform float u_loopStart;
uniform float u_loopEnd;
uniform int u_loopEnabled;

const vec3 BG_COLOR = vec3(0.102, 0.102, 0.102);  // #1a1a1a

in vec2 v_uv;
out vec4 fragColor;

void main() {
    // Max-pool the full 2D peak texture into the overview's pixel buckets.
    float samplesPerPixel = max(1.0, u_peakCount / u_resolution.x);
    float sampleCount = min(64.0, ceil(samplesPerPixel));
    float centerIndex = v_uv.x * u_peakCount;
    float firstIndex = centerIndex - samplesPerPixel * 0.5;
    float peak = 0.0;
    if (u_hasPeaks == 1) {
        for (int i = 0; i < 64; i++) {
            if (float(i) >= sampleCount) break;
            float sampleIndex = clamp(firstIndex + (float(i) + 0.5) * samplesPerPixel / sampleCount, 0.0, u_peakCount - 1.0);
            float texX = (mod(floor(sampleIndex), u_peakWidth) + 0.5) / u_peakWidth;
            float texY = (floor(sampleIndex / u_peakWidth) + 0.5) / u_peakHeight;
            peak = max(peak, texture(u_waveformTex, vec2(texX, texY)).r);
        }
    }
    
    // Distance from center
    float distFromCenter = abs(v_uv.y - 0.5) * 2.0;
    
    // Draw waveform
    if (distFromCenter < peak && u_hasPeaks == 1) {
        vec3 color = u_deckColor * 0.6; // Dimmer than main waveform
        fragColor = vec4(color, 0.8);
    } else {
        fragColor = vec4(BG_COLOR, 1.0);
    }

    bool inLoop = u_loopStart >= 0.0 && u_loopEnd > u_loopStart && v_uv.x >= u_loopStart && v_uv.x <= u_loopEnd;
    if (inLoop) {
        vec3 loopColor = u_loopEnabled == 1 ? vec3(0.12, 0.82, 0.42) : vec3(0.55, 0.65, 0.72);
        fragColor.rgb = mix(fragColor.rgb, loopColor, u_loopEnabled == 1 ? 0.34 : 0.14);
    }
    
    // Playhead line
    float playheadX = u_position * u_resolution.x;
    if (abs(gl_FragCoord.x - playheadX) < 1.5) {
        fragColor = vec4(1.0, 1.0, 1.0, 1.0);
    }
}
`;

/**
 * Cue point marker fragment shader
 */
export const djCuePointFragmentShader = `#version 300 es
precision highp float;

uniform vec2 u_resolution;
uniform float u_cuePosition;     // Cue position (0-1)
uniform float u_position;        // Current position (0-1)
uniform float u_visibleRange;    // Visible time range / duration
uniform vec3 u_color;            // Cue color (default: amber)
uniform float u_markerSize;      // Marker size

in vec2 v_uv;
out vec4 fragColor;

void main() {
    // Calculate cue X position on screen
    float relPos = (u_cuePosition - u_position) / u_visibleRange + 0.5;
    
    if (relPos < 0.0 || relPos > 1.0) {
        discard;
    }
    
    vec2 pixel = v_uv * u_resolution;
    float cueX = relPos * u_resolution.x;
    float dist = abs(pixel.x - cueX);
    
    // Vertical line
    if (dist < 1.5) {
        fragColor = vec4(u_color, 1.0);
        return;
    }
    
    // Triangle at top
    if (pixel.y < u_markerSize) {
        float triWidth = u_markerSize * (1.0 - pixel.y / u_markerSize);
        if (dist < triWidth) {
            fragColor = vec4(u_color, 1.0);
            return;
        }
    }
    
    discard;
}
`;
