/**
 * ShaderProgram
 * 
 * Utility class for compiling and managing WebGL shader programs.
 * Handles GLSL compilation, linking, uniform management, and attribute setup.
 * 
 * Features:
 * - Shader compilation with error reporting
 * - Program linking with validation
 * - Uniform location caching
 * - Attribute location lookup
 * - Automatic resource cleanup
 * 
 * @module ShaderProgram
 */

export interface ShaderSource {
    vertex: string;
    fragment: string;
}

/** Converts this project's GLSL 300 ES fragment shaders to the WebGL 1 form. */
export function toWebGL1FragmentShader(source: string): string {
    return source
        .replace(/^\s*#version\s+300\s+es\s*\n/m, '')
        .replace(/^\s*in\s+vec2\s+v_uv\s*;\s*$/m, 'varying vec2 v_uv;')
        .replace(/^\s*out\s+vec4\s+fragColor\s*;\s*$/m, '')
        .replace(/\bfragColor\b/g, 'gl_FragColor')
        .replace(/\btexture\s*\(/g, 'texture2D(');
}

export class ShaderProgram {
    readonly gl: WebGL2RenderingContext | WebGLRenderingContext;
    readonly program: WebGLProgram;
    private uniformLocations: Map<string, WebGLUniformLocation | null> = new Map();
    private attributeLocations: Map<string, number> = new Map();

    constructor(gl: WebGL2RenderingContext | WebGLRenderingContext, source: ShaderSource) {
        this.gl = gl;
        
        const vertexShader = this.compileShader(gl.VERTEX_SHADER, source.vertex);
        const fragmentShader = this.compileShader(gl.FRAGMENT_SHADER, source.fragment);
        
        this.program = this.linkProgram(vertexShader, fragmentShader);
        
        // Clean up individual shaders after linking
        gl.deleteShader(vertexShader);
        gl.deleteShader(fragmentShader);
    }

    /**
     * Compiles a shader from source
     */
    private compileShader(type: number, source: string): WebGLShader {
        const { gl } = this;
        const shader = gl.createShader(type);
        
        if (!shader) {
            throw new Error(`Failed to create shader of type ${type}`);
        }
        
        gl.shaderSource(shader, source);
        gl.compileShader(shader);
        
        if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
            const info = gl.getShaderInfoLog(shader);
            gl.deleteShader(shader);
            const shaderType = type === gl.VERTEX_SHADER ? 'vertex' : 'fragment';
            throw new Error(`Failed to compile ${shaderType} shader:\n${info}\n\nSource:\n${this.addLineNumbers(source)}`);
        }
        
        return shader;
    }

    /**
     * Links vertex and fragment shaders into a program
     */
    private linkProgram(vertexShader: WebGLShader, fragmentShader: WebGLShader): WebGLProgram {
        const { gl } = this;
        const program = gl.createProgram();
        
        if (!program) {
            throw new Error('Failed to create shader program');
        }
        
        gl.attachShader(program, vertexShader);
        gl.attachShader(program, fragmentShader);
        gl.linkProgram(program);
        
        if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
            const info = gl.getProgramInfoLog(program);
            gl.deleteProgram(program);
            throw new Error(`Failed to link shader program:\n${info}`);
        }
        
        return program;
    }

    /**
     * Adds line numbers to shader source for debugging
     */
    private addLineNumbers(source: string): string {
        return source.split('\n').map((line, i) => `${(i + 1).toString().padStart(3, ' ')}: ${line}`).join('\n');
    }

    /**
     * Activates this shader program
     */
    use(): void {
        this.gl.useProgram(this.program);
    }

    /**
     * Gets uniform location with caching
     */
    getUniformLocation(name: string): WebGLUniformLocation | null {
        if (!this.uniformLocations.has(name)) {
            const location = this.gl.getUniformLocation(this.program, name);
            this.uniformLocations.set(name, location);
        }
        return this.uniformLocations.get(name) ?? null;
    }

    /**
     * Gets attribute location with caching
     */
    getAttributeLocation(name: string): number {
        if (!this.attributeLocations.has(name)) {
            const location = this.gl.getAttribLocation(this.program, name);
            this.attributeLocations.set(name, location);
        }
        return this.attributeLocations.get(name) ?? -1;
    }

    // ==================== Uniform Setters ====================

    setUniform1f(name: string, value: number): void {
        const location = this.getUniformLocation(name);
        if (location !== null) {
            this.gl.uniform1f(location, value);
        }
    }

    setUniform2f(name: string, x: number, y: number): void {
        const location = this.getUniformLocation(name);
        if (location !== null) {
            this.gl.uniform2f(location, x, y);
        }
    }

    setUniform3f(name: string, x: number, y: number, z: number): void {
        const location = this.getUniformLocation(name);
        if (location !== null) {
            this.gl.uniform3f(location, x, y, z);
        }
    }

    setUniform4f(name: string, x: number, y: number, z: number, w: number): void {
        const location = this.getUniformLocation(name);
        if (location !== null) {
            this.gl.uniform4f(location, x, y, z, w);
        }
    }

    setUniform1i(name: string, value: number): void {
        const location = this.getUniformLocation(name);
        if (location !== null) {
            this.gl.uniform1i(location, value);
        }
    }

    setUniformMatrix4fv(name: string, transpose: boolean, value: Float32Array): void {
        const location = this.getUniformLocation(name);
        if (location !== null) {
            this.gl.uniformMatrix4fv(location, transpose, value);
        }
    }

    /**
     * Disposes of GPU resources
     */
    dispose(): void {
        this.gl.deleteProgram(this.program);
        this.uniformLocations.clear();
        this.attributeLocations.clear();
    }
}

/**
 * ShaderCache - Manages compiled shader programs
 */
export class ShaderCache {
    private cache: Map<string, ShaderProgram> = new Map();
    private gl: WebGL2RenderingContext | WebGLRenderingContext;

    constructor(gl: WebGL2RenderingContext | WebGLRenderingContext) {
        this.gl = gl;
    }

    /**
     * Gets or creates a shader program
     */
    getProgram(key: string, source: ShaderSource): ShaderProgram {
        if (!this.cache.has(key)) {
            const program = new ShaderProgram(this.gl, source);
            this.cache.set(key, program);
        }
        return this.cache.get(key)!;
    }

    /**
     * Checks if a program exists in the cache
     */
    has(key: string): boolean {
        return this.cache.has(key);
    }

    /**
     * Removes a program from the cache and disposes it
     */
    remove(key: string): void {
        const program = this.cache.get(key);
        if (program) {
            program.dispose();
            this.cache.delete(key);
        }
    }

    /**
     * Disposes all cached programs
     */
    dispose(): void {
        for (const program of this.cache.values()) {
            program.dispose();
        }
        this.cache.clear();
    }
}
