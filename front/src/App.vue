<template>
  <a-config-provider :locale="lang">
    <router-view />
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

  const lang = ref();

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
