/** Audio-reactive aurora scene for the album overlay and fullscreen background. */
import { sceneFragmentHeader } from './scene';

export const auroraFragmentShader = `${sceneFragmentHeader}
void main() {
    // Layered curtains with vertical folds, teal leading edges, and violet tops.
    vec2 p = scenePosition();
    float bass = energy(u_bass);
    float mid = energy(u_mid);
    vec3 teal = vec3(0.07, 1.0, 0.64);
    vec3 violet = vec3(0.62, 0.15, 1.0);
    vec3 light = vec3(0.0);
    for (int i = 0; i < 3; i++) {
        float layer = float(i);
        float phase = p.x * (5.7 + layer * 1.6) - u_time * (0.41 + layer * 0.11) + layer * 2.0;
        float floorY = -0.25 + layer * 0.11 + sin(phase) * (0.083 + 0.048 * bass)
            + sin(phase * 1.9 + layer) * 0.037;
        float rise = p.y - floorY;
        float folds = 0.38 + 0.62 * pow(0.5 + 0.5 * sin(phase * 7.0 + sin(phase * 2.0) * 2.2), 2.0);
        float height = 0.22 + 0.16 * band(abs(sin(phase * 0.31)) * 0.22) + mid * 0.11;
        float curtain = smoothstep(-0.008, 0.020, rise) * (1.0 - smoothstep(0.01, height, rise));
        vec3 tint = mix(teal, violet, smoothstep(0.035, height * 0.85, rise) * 0.72 + 0.10 * layer);
        light += tint * curtain * folds * 0.56;
        light += teal * filament(rise, 0.004) * folds * 0.22;
    }
    light += vec3(0.46, 0.68, 1.0) * starGrid(p, 31.0, 0.75) * 0.36;
    fragColor = sceneColor(light);
}
`;
