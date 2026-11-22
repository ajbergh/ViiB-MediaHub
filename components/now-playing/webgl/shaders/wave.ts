/** Audio-reactive wave scene for the album overlay and fullscreen background. */
import { sceneFragmentHeader } from './scene';

export const waveFragmentShader = `${sceneFragmentHeader}
void main() {
    // A braided waveform with travelling highlights and a mirrored reflection.
    float x = v_uv.x;
    float y = v_uv.y - 0.53;
    float bass = energy(u_bass);
    float envelope = sin(x * 3.14159);
    float trace = wave(x) * (0.22 + 0.21 * bass);
    float phase = x * 19.0 - u_time * (2.0 + bass);
    float twist = sin(phase) * (0.04 + bass * 0.10) * envelope;
    vec3 blue = vec3(0.12, 0.72, 1.0);
    vec3 pink = vec3(1.0, 0.13, 0.60);
    vec3 light = vec3(0.0);
    for (int i = 0; i < 3; i++) {
        float strand = float(i) - 1.0;
        float line = trace * (0.70 + 0.22 * float(i)) + twist * strand;
        vec3 tint = mix(blue, pink, 0.5 + 0.5 * sin(phase * 0.52 + strand));
        float shine = 0.30 + 0.34 * pow(0.5 + 0.5 * sin(phase + strand), 4.0);
        light += tint * filament(y - line, 0.004 + 0.002 * bass) * shine;
        light += tint * filament(y + 0.20 + line * 0.52, 0.004) * 0.16;
    }
    float gridX = abs(fract(x * 24.0) - 0.5);
    float gridY = abs(fract(y * 13.0) - 0.5);
    light += blue * (1.0 - smoothstep(0.0, 0.025, min(gridX, gridY))) * exp(-abs(y) * 6.5) * 0.085;
    light += pink * starGrid(scenePosition(), 26.0, 1.5) * (0.10 + energy(u_treble) * 0.42);
    fragColor = sceneColor(light);
}
`;
