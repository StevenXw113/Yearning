<script lang="ts" setup>
  // <a-app> 内部的桥接组件：useApp() 必须在 a-app 的**子组件**里调用才拿得到
  // 上下文实例（在提供者 App.vue 自身的 setup 里调用会得到缺方法的占位对象）。
  // 拿到实例后填进 antdFeedback 桥，axios 拦截器等非组件上下文由此获得
  // 与 <a-app> 同源、主题跟随的 message/notification。
  import { App } from 'ant-design-vue';
  import { bindAntdFeedback } from '@/config/antdApp';

  const { message, notification } = App.useApp();
  bindAntdFeedback({ message, notification });
</script>

<template>
  <span style="display: none" />
</template>
