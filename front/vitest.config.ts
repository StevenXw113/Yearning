import { defineConfig } from 'vitest/config';
import vue from '@vitejs/plugin-vue';
import * as path from 'path';

// 前端单测配置：happy-dom 提供 sessionStorage/DOM（store 持久化与 sanitize 需要），
// alias 与 vite.config.ts 保持一致。组件挂载测试用 @vue/test-utils（.vue 转换靠 vue 插件）。
export default defineConfig({
  plugins: [vue()],
  test: {
    environment: 'happy-dom',
    include: ['src/**/__tests__/**/*.test.ts'],
    // exFAT 盘上 macOS 会生成 ._* AppleDouble 垃圾文件（本仓已知怪癖），跳过
    exclude: ['**/node_modules/**', '**/._*'],
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src/'),
    },
  },
});
