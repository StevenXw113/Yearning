<template>
  <a-config-provider :locale="lang" :theme="themeConfig">
    <a-app>
      <router-view />
    </a-app>
  </a-config-provider>
</template>

<script lang="ts" setup>
  import zhCN from 'ant-design-vue/es/locale/zh_CN';
  import enUS from 'ant-design-vue/es/locale/en_US';
  import { locale } from 'dayjs';
  import 'dayjs/locale';
  import { onMounted, ref } from 'vue';
  import { systemLang } from './apis/loginApi';
  import i18n from '@/lang';
  import { themeConfig, applyTheme, isDark } from '@/style/theme';

  const lang = ref();

  onMounted(() => {
    // 主题在渲染前就位（data-theme 驱动 base.less 的 CSS 变量），避免闪烁
    applyTheme(isDark.value);
  });

  onMounted(async () => {
    const { data } = await systemLang();
    // 个人信息页可切换语言，本地选择优先于服务端配置（conf.toml 的 Lang 仅为默认值）
    const current = localStorage.getItem('lang') || data.payload;
    locale(current);
    sessionStorage.setItem('lang', current);
    i18n.global.locale.value = current;
    lang.value = current === 'en_US' ? enUS : zhCN;
  });
</script>
