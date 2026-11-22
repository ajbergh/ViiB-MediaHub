/** Audio-reactive stardust scene for the album overlay and fullscreen background. */
import { sceneFragmentHeader } from './scene';

export const stardustFragmentShader = `${sceneFragmentHeader}
void main() {
    // A tilted orbital halo with a nebula core and two layers of drifting stars.
    vec2 p = scenePosition();
    float angle = -0.46 + 0.06 * sin(u_time * 0.29);
    vec2 orbit = mat2(cos(angle), sin(angle), -sin(angle), cos(angle)) * p;
    orbit.y *= 2.18;
    float r = length(orbit);
    float a = atan(orbit.y, orbit.x + 0.00001);
    float bass = energy(u_bass);
    float dust = 0.5 + 0.5 * snoise(orbit * 14.0 + vec2(u_time * 0.19, -u_time * 0.11));
    vec3 violet = vec3(0.53, 0.20, 1.0);
    vec3 ice = vec3(0.27, 0.77, 1.0);
    vec3 light = violet * exp(-length(p) * 7.2) * (0.22 + bass * 0.30);
    for (int i = 0; i < 3; i++) {
        float layer = float(i);
        float radius = 0.20 + layer * 0.075 + bass * 0.035;
        float d = r - radius + sin(a * (3.0 + layer) - u_time + layer) * 0.012;
        vec3 tint = mix(violet, ice, layer * 0.5);
        light += tint * filament(d, 0.005 + layer * 0.0015) * (0.09 + dust * 0.39);
        light += tint * exp(-abs(d) * 32.0) * dust * 0.16;
    }
    float stars = starGrid(p + vec2(u_time * 0.011, 0.0), 18.0, 0.9);
    stars += starGrid(p - vec2(u_time * 0.006, 0.0), 33.0, 1.7) * 0.64;
    light += mix(ice, vec3(1.0), 0.45) * stars * (0.47 + energy(u_treble) * 0.37);
    fragColor = sceneColor(light);
}
`;
