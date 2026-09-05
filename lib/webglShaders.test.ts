import { describe, expect, it } from 'vitest';
import { toDJWebGL1FragmentShader } from '../components/dj/v2/webgl/DJWaveformShaders';
import { toWebGL1FragmentShader } from '../components/now-playing/webgl/ShaderProgram';

const webGL2Fragment = `#version 300 es
precision highp float;
in vec2 v_uv;
out vec4 fragColor;
uniform sampler2D source;
void main() { fragColor = texture(source, v_uv); }`;

describe('WebKit WebGL 1 shader conversion', () => {
  for (const convert of [toWebGL1FragmentShader, toDJWebGL1FragmentShader]) {
    it('converts GLSL 300 ES fragment syntax to WebGL 1 syntax', () => {
      const converted = convert(webGL2Fragment);

      expect(converted).not.toContain('#version 300 es');
      expect(converted).toContain('varying vec2 v_uv;');
      expect(converted).toContain('gl_FragColor = texture2D(source, v_uv);');
      expect(converted).not.toContain('out vec4 fragColor;');
    });
  }
});
