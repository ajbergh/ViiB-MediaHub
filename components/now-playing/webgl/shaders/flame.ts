/** Audio-reactive flame scene for the album overlay and fullscreen background. */
import { sceneFragmentHeader } from './scene';

export const flameFragmentShader = `${sceneFragmentHeader}
void main() {
    // A ring of fire. Frequency bins drive the tongues along its circumference.
    vec2 p = scenePosition();
    float r = length(p);
    float a = atan(p.y, p.x + 0.00001) / TAU + 0.5;
    float bass = energy(u_bass);
    float inner = 0.22 + 0.035 * bass;
    float bin = floor(a * 42.0);
    float strength = band(pow(bin / 42.0, 2.0) * 0.70);
    float curl = snoise(vec2(a * 29.0, r * 15.0 - u_time * 1.6));
    float tongue = 0.048 + strength * 0.19 + 0.027 * curl;
    float edge = r - inner;
    float lick = pow(0.5 + 0.5 * cos((a * 42.0 + curl * 0.29) * TAU), 1.45);
    float flame = smoothstep(-0.02, 0.018, edge)
        * (1.0 - smoothstep(tongue * 0.34, tongue + 0.012, edge)) * lick;
    vec3 red = vec3(1.0, 0.15, 0.035);
    vec3 gold = vec3(1.0, 0.69, 0.17);
    vec3 light = mix(gold, red, smoothstep(0.0, 0.13, edge)) * flame * 0.94;
    light += gold * filament(edge + 0.009, 0.004) * (0.43 + 0.40 * bass);
    light += red * exp(-abs(edge) * 12.0) * 0.23;
    vec2 ember = vec2(a * 2.0 + curl * 0.015, r - u_time * (0.065 + bass * 0.04));
    light += gold * starGrid(ember, 32.0, 2.2) * smoothstep(0.01, 0.12, edge) * exp(-edge * 2.0) * 0.60;
    fragColor = sceneColor(light);
}
`;
