import { writeFileSync } from 'node:fs';
import { fileURLToPath, URL } from 'node:url';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

const outDir = fileURLToPath(new URL('../backend/console/dist', import.meta.url));

/**
 * 管理后台：只打包 playground/console（不含接入文档），产物由 Go 服务 //go:embed 内嵌（见 backend/console）。
 * 本地开发不需要构建，pnpm start 后访问 http://localhost:8000/console/ 即可热更新调试。
 */
export default defineConfig({
  root: fileURLToPath(new URL('./playground/console', import.meta.url)),
  // 服务端可通过 CONSOLE_PATH 挂到任意路径下，资源一律使用相对地址
  base: './',
  publicDir: false,
  plugins: [
    react(),
    {
      // emptyOutDir 会清掉入库的 .gitkeep；未构建时 go:embed 依赖它保证目录存在
      name: 'keep-embed-placeholder',
      closeBundle() {
        writeFileSync(`${outDir}/.gitkeep`, '');
      },
    },
  ],
  resolve: {
    alias: {
      '@aetherlib/g-captcha': fileURLToPath(new URL('./src/index.tsx', import.meta.url)),
    },
  },
  build: {
    outDir,
    emptyOutDir: true,
  },
});
