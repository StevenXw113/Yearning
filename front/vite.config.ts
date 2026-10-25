import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import Components from 'unplugin-vue-components/vite';
import { AntDesignVueResolver } from 'unplugin-vue-components/resolvers';
import * as path from 'path';

// https://vitejs.dev/config/
export default defineConfig({
  base: '/front/',
  // build.minify 沿用 vite 8 默认(oxc):显式 'esbuild' 会要求单独安装 esbuild,
  // 且 vite 8 已把 esbuild 从内置依赖移除
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
  plugins: [
    vue(),
    // Antd 按需:全量 .use(Antd) 使整库进入口 chunk,tree-shaking 失效
    Components({ dts: false, resolvers: [AntDesignVueResolver({ importStyle: false })] }),
  ],
  build: {
    rollupOptions: {
      output: {
        // monaco 单拆成独立 chunk:只在用到编辑器的路由按需加载
        manualChunks(id) {
          if (id.includes('monaco-editor')) return 'monaco';
        },
      },
    },
  },
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
