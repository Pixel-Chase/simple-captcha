import { fileURLToPath, URL } from 'node:url';
import react from '@vitejs/plugin-react';
import { defineConfig, loadEnv } from 'vite';

const repoRoot = fileURLToPath(new URL('..', import.meta.url));
const playgroundRoot = fileURLToPath(new URL('./playground', import.meta.url));

// 本地演示页：包名直接指向 src 源码，保存即热更新，不依赖 dist 产物
export default defineConfig(({ mode }) => ({
  root: playgroundRoot,
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
    // 页面与 API 同源：/api/ 转发到验证码后端。保留浏览器的 Host（changeOrigin: false），
    // 后端据此判断同源请求，Origin 与 Host 一致。键必须带结尾斜杠，否则会把源码模块 /api.ts 也转发出去
    proxy: {
      '/api/': {
        target:
          loadEnv(mode, playgroundRoot, 'VITE_').VITE_API_PROXY_TARGET || 'http://127.0.0.1:3321',
        changeOrigin: false,
      },
    },
  },
  build: {
    outDir: fileURLToPath(new URL('./playground-dist', import.meta.url)),
    emptyOutDir: true,
  },
}));
