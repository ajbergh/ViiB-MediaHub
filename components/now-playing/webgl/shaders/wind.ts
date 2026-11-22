/** Audio-reactive wind scene for the album overlay and fullscreen background. */
import { sceneFragmentHeader } from './scene';

export const windFragmentShader = `${sceneFragmentHeader}
void main() {
    // Long-exposure streamlines curl around the center, showing flow and depth.
    vec2 p = scenePosition();
    float mid = energy(u_mid);
    float treble = energy(u_treble);
    float radius = length(p);
    float theta = 0.23 + 0.96 * exp(-radius * 2.6) + 0.21 * sin(u_time * 0.52);
    vec2 flow = mat2(cos(theta), sin(theta), -sin(theta), cos(theta)) * p;
    vec3 teal = vec3(0.14, 0.95, 0.82);
    vec3 amber = vec3(1.0, 0.62, 0.21);
    vec3 light = teal * exp(-radius * 4.1) * 0.09;
    for (int i = 0; i < 3; i++) {
        float layer = float(i);
        float curl = sin(flow.x * (4.0 + layer) + u_time * 0.55 + layer) * (0.043 + mid * 0.034);
        float track = (flow.y + curl) * (13.0 + layer * 6.0) + layer * 0.33;
        float row = floor(track);
        float across = abs(fract(track) - 0.5);
        float direction = flow.x * (3.0 + layer * 1.9) - u_time * (0.54 + layer * 0.23) + hash(row + layer * 23.0);
        float trail = pow(0.5 + 0.5 * cos(direction * TAU), 3.4);
        vec3 tint = mix(teal, amber, layer * 0.28 + 0.12 * sin(row));
        light += tint * filament(across, 0.058) * trail * (0.22 + mid * 0.30);
    }
    light += amber * starGrid(flow - vec2(u_time * 0.11, 0.0), 30.0, 1.5) * (0.16 + treble * 0.45);
    fragColor = sceneColor(light);
}
`;
