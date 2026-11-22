/** Canvas versions of the audio-reactive scenes, drawn at CSS-pixel resolution. */
import type { VisualizerMode } from '../../types';

const TAU = Math.PI * 2;
const ICE = 'rgb(114, 223, 255)';
const CYAN = 'rgb(36, 186, 250)';
const PINK = 'rgb(255, 54, 141)';
const VIOLET = 'rgb(155, 91, 255)';
const JADE = 'rgb(69, 255, 193)';
const GOLD = 'rgb(255, 208, 121)';

interface Scene {
    ctx: CanvasRenderingContext2D;
    width: number;
    height: number;
    scale: number;
    time: number;
    bass: number;
    mid: number;
    treble: number;
    frequency: Uint8Array;
    waveform: Uint8Array;
}

const fraction = (value: number) => value - Math.floor(value);
const seed = (index: number) => fraction(Math.sin(index * 127.1 + 31.7) * 43758.5453);
const frequency = (data: Uint8Array, position: number) =>
    Math.sqrt(data[Math.round(position * (data.length - 1))] / 255);
const waveform = (data: Uint8Array, position: number) => {
    const index = position * (data.length - 1);
    const start = Math.floor(index);
    const mix = index - start;
    return (data[start] * (1 - mix) + data[Math.min(start + 1, data.length - 1)] * mix - 128) / 128;
};

function average(data: Uint8Array, from: number, to: number) {
    let sum = 0;
    for (let i = from; i < to; i++) sum += data[i];
    return Math.sqrt(sum / ((to - from) * 255));
}

function plot(ctx: CanvasRenderingContext2D, steps: number, point: (position: number) => [number, number]) {
    ctx.beginPath();
    for (let i = 0; i <= steps; i++) {
        const [x, y] = point(i / steps);
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
    }
}

function stroke(ctx: CanvasRenderingContext2D, color: string | CanvasGradient, width: number, alpha: number) {
    ctx.strokeStyle = color;
    ctx.lineWidth = width;
    ctx.globalAlpha = alpha;
    ctx.stroke();
}

function glow(ctx: CanvasRenderingContext2D, x: number, y: number, radius: number, color: string, alpha: number) {
    const brush = ctx.createRadialGradient(x, y, 0, x, y, radius);
    brush.addColorStop(0, color);
    brush.addColorStop(1, 'transparent');
    ctx.globalAlpha = alpha;
    ctx.fillStyle = brush;
    ctx.fillRect(x - radius, y - radius, radius * 2, radius * 2);
}

function stars(scene: Scene, count: number, speed: number, color: string, alpha: number) {
    const { ctx, width, height, scale, time, treble } = scene;
    ctx.fillStyle = color;
    for (let i = 0; i < count; i++) {
        const x = width * fraction(seed(i) + time * speed * (0.5 + seed(i + 99)));
        const y = height * seed(i + 207);
        const pulse = 0.5 + 0.5 * Math.sin(time * 1.9 + i * 2.4);
        const size = Math.max(0.55, scale * (0.0012 + seed(i + 431) * 0.002));
        ctx.globalAlpha = alpha * (0.28 + pulse * 0.39 + treble * 0.33);
        ctx.fillRect(x, y, size, size);
        if (pulse + treble > 1.26 && i % 5 === 0) {
            ctx.fillRect(x - size, y + size / 3, size * 3, size / 3);
            ctx.fillRect(x + size / 3, y - size, size / 3, size * 3);
        }
    }
}

function drawWave(scene: Scene) {
    const { ctx, width, height, scale, time, bass, waveform: data } = scene;
    const center = height * 0.47;
    const tint = ctx.createLinearGradient(0, 0, width, 0);
    tint.addColorStop(0, CYAN);
    tint.addColorStop(0.38, VIOLET);
    tint.addColorStop(0.72, PINK);
    tint.addColorStop(1, ICE);
    glow(ctx, width * 0.52, center, Math.max(width * 0.49, scale * 0.32), VIOLET, 0.16);
    stars(scene, 100, 0.018, ICE, 0.46);
    ctx.beginPath();
    for (let i = 1; i < 24; i++) {
        ctx.moveTo(width * i / 24, height * 0.29);
        ctx.lineTo(width * i / 24, height * 0.74);
    }
    for (let i = 0; i <= 8; i++) {
        const y = height * (0.29 + i * 0.45 / 8);
        ctx.moveTo(0, y);
        ctx.lineTo(width, y);
    }
    stroke(ctx, CYAN, 0.75, 0.17);
    for (let strand = -1; strand <= 1; strand++) {
        const line = (x: number) => scale * (
            waveform(data, x) * (0.23 + bass * 0.19) * (0.95 + strand * 0.20)
            + Math.sin(x * 19 - time * (2 + bass)) * (0.042 + bass * 0.095) * Math.sin(Math.PI * x) * strand
        );
        plot(ctx, 180, (x) => [width * x, center + scale * 0.21 + line(x) * 0.42]);
        stroke(ctx, tint, scale * 0.006, 0.28);
        plot(ctx, 180, (x) => [width * x, center - line(x)]);
        stroke(ctx, tint, scale * (0.013 + bass * 0.015), 0.31);
        stroke(ctx, tint, Math.max(1, scale * 0.004), 0.97);
    }
}

function drawFlame(scene: Scene) {
    const { ctx, width, height, scale, time, bass, treble, frequency: data } = scene;
    const x = width / 2;
    const y = height / 2;
    const inner = scale * (0.22 + bass * 0.045);
    glow(ctx, x, y, scale * 0.52, 'rgb(214, 49, 32)', 0.30 + bass * 0.16);
    const fire = ctx.createRadialGradient(x, y, inner * 0.92, x, y, scale * 0.51);
    fire.addColorStop(0, 'rgb(255, 242, 186)');
    fire.addColorStop(0.20, 'rgb(255, 177, 47)');
    fire.addColorStop(0.58, 'rgb(255, 62, 19)');
    fire.addColorStop(1, 'transparent');
    const flameCount = 54;
    ctx.fillStyle = fire;
    ctx.globalAlpha = 0.88;
    for (let i = 0; i < flameCount; i++) {
        const angle = i * TAU / flameCount;
        const response = frequency(data, 0.70 * (i / flameCount) ** 2);
        const length = scale * (0.052 + seed(i) * 0.066 + response * 0.18);
        const bend = Math.sin(time * 3.1 + i * 1.18) * 0.13;
        ctx.save();
        ctx.translate(x, y);
        ctx.rotate(angle);
        ctx.beginPath();
        ctx.moveTo(inner - 2, -scale * 0.018);
        ctx.bezierCurveTo(inner + length * 0.48, -scale * 0.032, inner + length * 0.89, bend * length, inner + length, bend * length * 2);
        ctx.bezierCurveTo(inner + length * 0.60, bend * length + scale * 0.032, inner + length * 0.45, scale * 0.048, inner - 2, scale * 0.018);
        ctx.restore();
        ctx.fill();
    }
    ctx.beginPath();
    ctx.arc(x, y, inner, 0, TAU);
    stroke(ctx, GOLD, scale * 0.012, 0.29);
    stroke(ctx, 'rgb(255, 240, 189)', Math.max(1.1, scale * 0.003), 0.96);
    ctx.fillStyle = GOLD;
    for (let i = 0; i < 96; i++) {
        const age = fraction(seed(i + 200) + time * (0.17 + seed(i) * 0.14));
        const a = seed(i + 50) * TAU + age * Math.sin(i) * 0.44;
        const r = inner + age * scale * (0.22 + bass * 0.22);
        const size = Math.max(0.8, (1 - age) * scale * 0.006);
        ctx.globalAlpha = (1 - age) * (0.47 + treble * 0.48);
        ctx.fillRect(x + Math.cos(a) * r, y + Math.sin(a) * r, size, size);
    }
}

function drawStardust(scene: Scene) {
    const { ctx, width, height, scale, time, bass, treble } = scene;
    const x = width / 2;
    const y = height / 2;
    stars(scene, 180, 0.006, ICE, 0.78);
    glow(ctx, x, y, scale * 0.49, 'rgb(87, 41, 191)', 0.31);
    glow(ctx, x, y, scale * (0.19 + bass * 0.10), 'rgb(152, 105, 255)', 0.36);
    ctx.save();
    ctx.translate(x, y);
    ctx.rotate(-0.44 + 0.06 * Math.sin(time * 0.29));
    for (let ring = 0; ring < 4; ring++) {
        const radius = scale * (0.17 + ring * 0.067 + bass * 0.041);
        const color = [VIOLET, 'rgb(189, 118, 255)', 'rgb(124, 156, 255)', ICE][ring];
        ctx.beginPath();
        ctx.ellipse(0, 0, radius, radius * 0.46, 0, 0, TAU);
        stroke(ctx, color, scale * 0.025, 0.13);
        stroke(ctx, color, Math.max(0.75, scale * 0.0028), 0.55);
        ctx.fillStyle = color;
        for (let i = 0; i < 47; i++) {
            const a = i * TAU / 47 + time * (0.17 + ring * 0.07);
            const spread = radius + scale * (seed(i + ring * 60) - 0.5) * 0.053;
            const size = Math.max(0.8, scale * (0.0018 + seed(i + 89) * 0.005));
            const glint = 0.5 + 0.5 * Math.cos(a * 3 - time + i);
            ctx.globalAlpha = 0.30 + glint * (0.36 + treble * 0.2);
            ctx.fillRect(Math.cos(a) * spread, Math.sin(a) * spread * 0.46, size * 1.7, size);
        }
    }
    ctx.restore();
}

function drawAurora(scene: Scene) {
    const { ctx, width, height, scale, time, bass, mid, frequency: data } = scene;
    stars(scene, 135, 0.002, ICE, 0.63);
    glow(ctx, width / 2, height * 0.67, Math.max(width * 0.47, scale * 0.35), JADE, 0.18);
    const tints = [JADE, 'rgb(120, 216, 255)', 'rgb(182, 122, 255)'];
    for (let layer = 0; layer < tints.length; layer++) {
        const color = tints[layer];
        const baseline = (x: number) => height * (0.75 - layer * 0.105)
            + scale * (Math.sin(x * (7.5 + layer * 2) - time * (0.58 + layer * 0.15) + layer * 2.1) * (0.077 + bass * 0.065)
                + Math.sin(x * 18.2 - time * 0.6 + layer) * 0.038);
        const curtain = ctx.createLinearGradient(0, height * 0.70, 0, height * 0.16);
        curtain.addColorStop(0, color);
        curtain.addColorStop(0.46, layer === 0 ? color : VIOLET);
        curtain.addColorStop(1, 'transparent');
        for (let i = 0; i <= 100; i++) {
            const x = i / 100;
            const floor = baseline(x);
            const phase = x * 49 + Math.sin(x * 22 - time * 1.5) * 2.25 + layer * 1.95 - time;
            const fold = (0.5 + 0.5 * Math.sin(phase)) ** 2;
            const rise = scale * (0.10 + mid * 0.15 + fold * 0.16 + frequency(data, 0.05 + x * 0.25) * 0.12);
            ctx.beginPath();
            ctx.moveTo(width * x, floor);
            ctx.lineTo(width * x + Math.cos(x * 8 - time) * rise * 0.29, floor - rise);
            stroke(ctx, curtain, width / 73, 0.11 + fold * 0.25);
        }
        plot(ctx, 128, (x) => [width * x, baseline(x)]);
        stroke(ctx, color, scale * 0.014, 0.16);
        stroke(ctx, color, Math.max(1, scale * 0.003), 0.61);
    }
}

function drawElectric(scene: Scene) {
    const { ctx, width, height, scale, time, bass, treble, waveform: data } = scene;
    glow(ctx, width / 2, height / 2, Math.max(width * 0.47, scale * 0.3), 'rgb(80, 70, 206)', 0.30 + bass * 0.17);
    const frame = Math.floor(time * 15);
    const channel = (x: number) => height * 0.5 + scale * (
        0.12 * Math.sin(x * 8 - time) + waveform(data, x) * 0.18
        + (seed(Math.round(x * 55) + frame * 57) - 0.5) * (0.045 + bass * 0.14)
    );
    for (let branch = 0; branch < 9; branch++) {
        const start = 0.09 + branch * 0.102 + (seed(frame + branch) - 0.5) * 0.06;
        const direction = branch % 2 === 0 ? 1 : -1;
        const y = channel(start);
        const reach = scale * (0.11 + seed(branch + frame * 9) * 0.19 + treble * 0.16);
        plot(ctx, 12, (step) => [
            width * start + reach * step * direction + (seed(step * 12 + branch * 30 + frame) - 0.5) * step * scale * 0.12,
            y - reach * direction * step * 0.7,
        ]);
        stroke(ctx, VIOLET, scale * 0.013, 0.26);
        stroke(ctx, ICE, Math.max(0.8, scale * 0.0025), 0.58 + treble * 0.32);
    }
    plot(ctx, 120, (x) => [width * x, channel(x)]);
    stroke(ctx, CYAN, scale * (0.026 + bass * 0.027), 0.33);
    stroke(ctx, ICE, scale * 0.009, 0.84);
    stroke(ctx, 'rgb(239, 252, 255)', Math.max(0.85, scale * 0.0037), 0.97);
    stars(scene, 96, 0.048, ICE, 0.61);
}

function drawGrass(scene: Scene) {
    const { ctx, width, height, scale, time, bass, waveform: data } = scene;
    const horizon = height * 0.55;
    glow(ctx, width * 0.51, horizon, Math.max(width * 0.49, scale * 0.35), 'rgb(72, 185, 105)', 0.38);
    const colors = ['rgb(100, 211, 161)', 'rgb(104, 198, 104)', 'rgb(139, 209, 99)', 'rgb(194, 239, 126)'];
    for (let row = 0; row < colors.length; row++) {
        const count = 105 - row * 12;
        const ground = horizon + row * (height * 0.142);
        for (let blade = 0; blade <= count; blade++) {
            const at = blade / count;
            const shade = seed(blade + row * 200);
            const x = at * width;
            const length = scale * (0.025 + shade * 0.12 + row * 0.022 + Math.abs(waveform(data, at)) * 0.20 + bass * 0.089);
            const bend = scale * (0.016 + row * 0.015) * Math.sin(at * 13 - time * 1.4 + row * 0.5) + length * 0.28;
            ctx.beginPath();
            ctx.moveTo(x - 1, ground);
            ctx.quadraticCurveTo(x + bend * 0.2, ground - length * 0.57, x + bend, ground - length);
            ctx.quadraticCurveTo(x + bend * 0.47, ground - length * 0.48, x + scale * (0.004 + row * 0.002), ground);
            ctx.globalAlpha = 0.21 + row * 0.12 + shade * 0.23;
            ctx.fillStyle = colors[row];
            ctx.fill();
        }
    }
    stars(scene, 91, 0.022, 'rgb(228, 255, 162)', 0.64);
}

function drawTunnel(scene: Scene) {
    const { ctx, width, height, scale, time, bass, waveform: data } = scene;
    const x = width / 2 + scale * 0.062 * Math.sin(time * 0.48);
    const y = height / 2 + scale * 0.048 * Math.cos(time * 0.39);
    const edge = Math.hypot(width, height) * 0.61;
    glow(ctx, x, y, scale * 0.44, 'rgb(110, 40, 113)', 0.43 + bass * 0.19);
    for (let rail = 0; rail < 16; rail++) {
        const angle = rail * TAU / 16 - time * 0.42;
        plot(ctx, 22, (step) => {
            const r = step * edge;
            const a = angle + r / scale * 2.15;
            return [x + Math.cos(a) * r, y + Math.sin(a) * r];
        });
        stroke(ctx, CYAN, Math.max(1, scale * 0.0033), 0.31);
    }
    const travel = time * (1.18 + bass * 0.22);
    for (let i = 13; i >= 0; i--) {
        const depth = (i + fraction(travel)) / 13;
        const radius = edge * depth ** 2.3;
        const tint = (i + Math.floor(travel)) % 2 === 0 ? CYAN : PINK;
        plot(ctx, 112, (step) => {
            const angle = step * TAU;
            const sample = waveform(data, step);
            const deformation = 1 + 0.059 * Math.cos(angle * 6 + time * 0.5) + sample * 0.19;
            return [x + Math.cos(angle) * radius * deformation, y + Math.sin(angle) * radius * deformation];
        });
        ctx.closePath();
        stroke(ctx, tint, Math.max(1.1, scale * depth * 0.033), depth * 0.21);
        stroke(ctx, tint, Math.max(0.75, scale * depth * 0.005), 0.40 + depth * 0.50);
    }
}

function drawWind(scene: Scene) {
    const { ctx, width, height, scale, time, mid, treble } = scene;
    const x = width / 2;
    const y = height / 2;
    const edge = Math.hypot(width, height);
    glow(ctx, x, y, scale * 0.58, 'rgb(36, 119, 119)', 0.25);
    const colors = [JADE, 'rgb(77, 207, 234)', 'rgb(255, 182, 106)'];
    for (let layer = 0; layer < colors.length; layer++) {
        for (let row = 0; row < 21; row++) {
            const shift = (row - 10 + layer * 0.29) * scale * 0.075;
            const front = fraction(seed(row + layer * 20) + time * (0.12 + layer * 0.057));
            const length = 0.24 + seed(row + 179) * 0.19 + mid * 0.22;
            plot(ctx, 19, (step) => {
                const travel = front * 1.55 - 0.29 + step * length;
                const px = (travel - 0.5) * edge;
                const py = shift + Math.sin(px / scale * (3.6 + layer * 0.55) + time * 0.82 + layer) * scale * (0.052 + mid * 0.061);
                const turn = -0.26 - 0.84 * Math.exp(-Math.hypot(px, py) / scale * 2.2) + Math.sin(time * 0.45) * 0.15;
                return [x + px * Math.cos(turn) - py * Math.sin(turn), y + px * Math.sin(turn) + py * Math.cos(turn)];
            });
            stroke(ctx, colors[layer], scale * (0.009 + layer * 0.004), 0.12);
            stroke(ctx, colors[layer], Math.max(0.75, scale * (0.002 + layer * 0.0009)), 0.42 + treble * 0.22);
        }
    }
    stars(scene, 72, 0.093, GOLD, 0.49);
}

type SceneMode = Exclude<VisualizerMode, 'OFF' | 'SPECTRUM' | 'FIREFLY_FIELD' | 'MILKDROP'>;

const scenes: Record<SceneMode, (scene: Scene) => void> = {
    WAVE: drawWave,
    FLAME_SPECTRUM: drawFlame,
    STARDUST_HALO: drawStardust,
    AURORA_RIBBON: drawAurora,
    ELECTRIC_ARC: drawElectric,
    GRASS_OSCILLOSCOPE: drawGrass,
    TUNNEL_WAVEFORM: drawTunnel,
    WIND_FIELD: drawWind,
};

export function drawVisualizerScene(
    ctx: CanvasRenderingContext2D,
    mode: SceneMode,
    width: number,
    height: number,
    timestamp: number,
    frequency: Uint8Array,
    waveform: Uint8Array,
) {
    if (width < 4 || height < 4) return;
    ctx.save();
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    scenes[mode]({
        ctx, width, height, frequency, waveform,
        scale: Math.min(width, height),
        time: timestamp / 1000,
        bass: average(frequency, 0, 30),
        mid: average(frequency, 30, 150),
        treble: average(frequency, 150, 300),
    });
    ctx.restore();
}
