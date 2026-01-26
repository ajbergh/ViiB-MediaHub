/**
 * Common Vertex Shader
 * 
 * Simple fullscreen quad vertex shader for fragment-shader-based visualizations.
 * Outputs UV coordinates (0-1) and clip-space position.
 */
export const commonVertexShader = `#version 300 es
precision highp float;

// Fullscreen quad vertices (two triangles)
// Position attribute: vec2 (-1 to 1)
in vec2 a_position;

out vec2 v_uv;

void main() {
    // Convert from clip space (-1 to 1) to UV space (0 to 1)
    v_uv = a_position * 0.5 + 0.5;
    gl_Position = vec4(a_position, 0.0, 1.0);
}
`;

/**
 * Fullscreen Quad Vertex Shader (WebGL 1 compatible)
 */
export const commonVertexShaderWebGL1 = `
precision highp float;

attribute vec2 a_position;

varying vec2 v_uv;

void main() {
    v_uv = a_position * 0.5 + 0.5;
    gl_Position = vec4(a_position, 0.0, 1.0);
}
`;

/**
 * Instanced Vertex Shader
 * 
 * For rendering many instances (particles, bars) with per-instance data.
 */
export const instancedVertexShader = `#version 300 es
precision highp float;

// Per-vertex attributes
in vec2 a_position;
in vec2 a_uv;

// Per-instance attributes
in vec2 a_instancePosition;
in vec2 a_instanceScale;
in vec4 a_instanceColor;
in float a_instanceRotation;

uniform vec2 u_resolution;
uniform float u_time;

out vec2 v_uv;
out vec4 v_color;

mat2 rotate2D(float angle) {
    float c = cos(angle);
    float s = sin(angle);
    return mat2(c, -s, s, c);
}

void main() {
    // Apply instance transform
    vec2 pos = a_position * a_instanceScale;
    pos = rotate2D(a_instanceRotation) * pos;
    pos += a_instancePosition;
    
    // Convert to clip space
    vec2 clipPos = (pos / u_resolution) * 2.0 - 1.0;
    clipPos.y = -clipPos.y; // Flip Y for canvas coordinates
    
    gl_Position = vec4(clipPos, 0.0, 1.0);
    v_uv = a_uv;
    v_color = a_instanceColor;
}
`;

/**
 * Point Sprite Vertex Shader
 * 
 * For GPU-efficient particle rendering using point sprites.
 */
export const pointSpriteVertexShader = `#version 300 es
precision highp float;

in vec2 a_position;
in float a_size;
in vec4 a_color;
in float a_life;

uniform vec2 u_resolution;
uniform float u_time;

out vec4 v_color;
out float v_life;

void main() {
    // Convert to clip space
    vec2 clipPos = (a_position / u_resolution) * 2.0 - 1.0;
    clipPos.y = -clipPos.y;
    
    gl_Position = vec4(clipPos, 0.0, 1.0);
    gl_PointSize = a_size;
    
    v_color = a_color;
    v_life = a_life;
}
`;

/**
 * Fullscreen quad geometry (two triangles covering screen)
 */
export const FULLSCREEN_QUAD_VERTICES = new Float32Array([
    -1, -1,  // bottom-left
     1, -1,  // bottom-right
    -1,  1,  // top-left
     1, -1,  // bottom-right
     1,  1,  // top-right
    -1,  1   // top-left
]);

/**
 * Unit quad with UVs for instanced rendering
 */
export const UNIT_QUAD_VERTICES = new Float32Array([
    // position (x, y), uv (u, v)
    -0.5, -0.5,  0, 0,
     0.5, -0.5,  1, 0,
    -0.5,  0.5,  0, 1,
     0.5, -0.5,  1, 0,
     0.5,  0.5,  1, 1,
    -0.5,  0.5,  0, 1
]);
