import { fileURLToPath, URL } from 'node:url';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

// 本地演示页：包名直接指向 src 源码，保存即热更新，不依赖 dist 产物
export default defineConfig({
  root: fileURLToPath(new URL('./playground', import.meta.url)),
  plugins: [react()],
  resolve: {
    alias: {
      '@aetherlib/g-captcha': fileURLToPath(new URL('./src/index.tsx', import.meta.url)),
    },
  },
  server: {
    port: 8000,
  },
  build: {
    outDir: fileURLToPath(new URL('./playground-dist', import.meta.url)),
    emptyOutDir: true,
  },
});
