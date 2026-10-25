import { defineConfig } from 'tsdown';

// 库构建：单入口打包为 ESM + CJS，自动外置 dependencies / peerDependencies（react、react-dom、axios）
export default defineConfig({
  entry: ['src/index.tsx'],
  format: ['esm', 'cjs'],
  platform: 'browser',
  target: 'es2020',
  tsconfig: 'tsconfig.build.json',
  dts: true,
  clean: true,
});
