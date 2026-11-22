/** Audio-reactive grass scene for the album overlay and fullscreen background. */
import { sceneFragmentHeader } from './scene';

export const grassFragmentShader = `${sceneFragmentHeader}
void main() {
    // Backlit rows of grass follow the waveform and recede toward a soft horizon.
    vec2 p = scenePosition();
    float x = v_uv.x;
    float bass = energy(u_bass);
    vec3 light = vec3(0.09, 0.33, 0.25) * exp(-abs(p.y + 0.055) * 10.0) * 0.55;
    for (int i = 0; i < 4; i++) {
        float row = float(i);
        float ground = -0.12 - row * 0.135;
        float blades = 88.0 - row * 15.0;
        float rise = p.y - ground;
        float bend = rise * rise * (2.1 * sin(x * 9.0 - u_time * 1.4 + row) + 1.4);
        float slot = (x - bend / max(u_resolution.x / u_resolution.y, 1.0)) * blades + row * 0.49;
        float id = floor(slot);
        float sampleX = clamp(id / blades, 0.0, 1.0);
        float blade = 0.07 + hash(id + row * 71.0) * 0.12 + abs(wave(sampleX)) * 0.17 + bass * 0.065;
        float tip = clamp(rise / blade, 0.0, 1.0);
        float silhouette = 1.0 - smoothstep(0.04, 0.22 * (1.0 - tip) + 0.045, abs(fract(slot) - 0.5));
        float mask = smoothstep(-0.014, 0.008, rise) * (1.0 - smoothstep(blade * 0.87, blade, rise));
        vec3 leaf = mix(vec3(0.035, 0.30, 0.24), vec3(0.59, 0.96, 0.32), tip);
        light += leaf * mask * silhouette * (0.34 + row * 0.12);
    }
    light += vec3(0.67, 0.96, 0.36) * starGrid(p - vec2(u_time * 0.013, u_time * 0.016), 24.0, 1.8)
        * (0.20 + energy(u_treble) * 0.48);
    fragColor = sceneColor(light);
}
`;
