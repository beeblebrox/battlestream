import typescript from '@rollup/plugin-typescript';
import { nodeResolve } from '@rollup/plugin-node-resolve';
import copy from 'rollup-plugin-copy';

const DIST = 'dist/com.battlestream.streamdeck.sdPlugin';

export default {
  input: 'src/plugin.ts',
  output: {
    file: `${DIST}/bin/plugin.js`,
    format: 'esm',
    sourcemap: true,
  },
  external: ['@napi-rs/canvas', 'ws'],
  plugins: [
    nodeResolve({ preferBuiltins: true }),
    typescript({ tsconfig: './tsconfig.json', declaration: false, declarationMap: false }),
    copy({
      targets: [
        { src: 'manifest.json', dest: DIST },
        { src: 'ui', dest: DIST },
        { src: 'imgs', dest: DIST },
        { src: 'profiles', dest: DIST },
      ],
      hook: 'writeBundle',
    }),
  ],
};
