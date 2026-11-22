/** Audio-reactive tunnel scene for the album overlay and fullscreen background. */
import { sceneFragmentHeader } from './scene';

export const tunnelFragmentShader = `${sceneFragmentHeader}
void main() {
    // Expanding waveform gates and perspectival rails lead into a moving vanishing point.
    float bass = energy(u_bass);
    vec2 p = scenePosition() - vec2(sin(u_time * 0.48) * 0.062, cos(u_time * 0.39) * 0.048);
    float a = atan(p.y, p.x + 0.00001);
    float r = length(p);
    float audio = wave(a / TAU + 0.5);
    float sides = 1.0 + 0.07 * cos(a * 6.0 + u_time * 0.48);
    float depth = max(0.02, r * sides * (1.0 + audio * 0.23));
    float progress = 0.23 / depth + u_time * 1.33;
    float line = abs(fract(progress) - 0.5);
    float bend = a + r * 2.85 - u_time * 0.42;
    float rail = abs(fract(bend / TAU * 16.0) - 0.5);
    vec3 cyan = vec3(0.10, 0.86, 1.0);
    vec3 fuchsia = vec3(1.0, 0.10, 0.44);
    vec3 tint = mix(cyan, fuchsia, 0.5 + 0.5 * sin(floor(progress) * 1.28 + u_time * 0.4));
    float fade = smoothstep(0.04, 0.14, r);
    vec3 light = tint * filament(line, 0.039 + bass * 0.035) * fade * 0.75;
    light += cyan * filament(rail, 0.021) * fade * 0.30;
    light += fuchsia * exp(-r * 10.0) * (0.32 + bass * 0.40);
    fragColor = sceneColor(light);
}
`;
