import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import * as path from 'path';

// https://vitejs.dev/config/
export default defineConfig({
  base: '/front/',
  build: {
    minify: 'esbuild',
  },
  server: {
    proxy: {
      '/register': {
        target: 'http://127.0.0.1:8000',
      },
      '/_next': {
        target: 'http://127.0.0.1:8000',
      },
      '/chatbot': {
        target: 'http://127.0.0.1:8000',
      },
      '/fetch': {
        target: 'http://127.0.0.1:8000',
      },
      '/lang': {
        target: 'http://127.0.0.1:8000',
      },
      '/login': {
        target: 'http://127.0.0.1:8000',
      },
      '/api': {
        target: 'http://127.0.0.1:8000',
        // 查询执行、工单/审计列表、详情时间线都走 WebSocket，不开 ws 时
        // 升级请求不会被转发，前端只会报「WebSocket连接失败」
        ws: true,
      },
      '/ldap': {
        target: 'http://127.0.0.1:8000',
      },
      '/downlaod/*': {
        target: 'http://127.0.0.1:8000',
      },
      '/oidc/state': {
        target: 'http://127.0.0.1:8000',
      },
    },
  },
  plugins: [vue()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src/'),
    },
  },
  css: {
    preprocessorOptions: {
      less: {
        javascriptEnabled: true,
      },
    },
  },
});
