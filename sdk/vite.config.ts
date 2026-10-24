import { fileURLToPath, URL } from 'node:url';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

const repoRoot = fileURLToPath(new URL('..', import.meta.url));

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
    // 接入文档页以 ?raw 读取仓库根目录的 README.md，该文件在 sdk 工作区之外，开发服务器需显式放行
    fs: {
      allow: [repoRoot],
    },
  },
  build: {
    outDir: fileURLToPath(new URL('./playground-dist', import.meta.url)),
    emptyOutDir: true,
  },
});
