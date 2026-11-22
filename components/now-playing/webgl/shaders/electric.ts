/** Audio-reactive electric scene for the album overlay and fullscreen background. */
import { sceneFragmentHeader } from './scene';

export const electricFragmentShader = `${sceneFragmentHeader}
void main() {
    // A plasma channel and branching leaders, with the strongest kicks widening the core.
    vec2 p = scenePosition();
    float x = v_uv.x;
    float bass = energy(u_bass);
    float treble = energy(u_treble);
    float jagged = snoise(vec2(x * 24.0, u_time * 2.5));
    float channel = 0.11 * sin(x * 8.0 - u_time * 0.9)
        + jagged * (0.054 + 0.12 * bass) + wave(x) * 0.16;
    vec3 cyan = vec3(0.12, 0.74, 1.0);
    vec3 violet = vec3(0.55, 0.23, 1.0);
    float main = p.y - channel;
    vec3 light = cyan * filament(main, 0.004 + bass * 0.004) * (0.55 + bass * 0.32);
    light += vec3(0.70, 0.92, 1.0) * exp(-abs(main) * 340.0) * 0.68;
    for (int i = 0; i < 4; i++) {
        float branch = float(i);
        float start = 0.10 + branch * 0.19 + sin(u_time * 1.8 + branch * 3.0) * 0.04;
        float progress = (x - start) * (mod(branch, 2.0) * 2.0 - 1.0);
        float reach = smoothstep(0.0, 0.032, progress) * (1.0 - smoothstep(0.10, 0.28, progress));
        float side = step(1.5, branch) * 2.0 - 1.0;
        float d = main - side * (progress * (0.69 + branch * 0.12) + jagged * 0.028);
        light += violet * filament(d, 0.0035) * reach * (0.27 + 0.51 * treble);
    }
    light += cyan * starGrid(p + vec2(-u_time * 0.045, 0.0), 33.0, 4.0)
        * exp(-abs(main) * 8.0) * 0.37;
    fragColor = sceneColor(light);
}
`;
