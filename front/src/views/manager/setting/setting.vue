<template>
  <PageHeader :title="title.title" :sub-title="title.subTitle"></PageHeader>
  <a-card>
    <a-tabs v-model:activeKey="activeKey">
      <a-tab-pane key="basic" :tab="$t('setting.basic')"><Basic /></a-tab-pane>
      <a-tab-pane key="advanced" :tab="$t('setting.adv')"
        ><Advanced
      /></a-tab-pane>
      <a-tab-pane key="ai" tab="AI"> <AI /></a-tab-pane>
    </a-tabs>
  </a-card>
</template>

<script lang="ts" setup>
  import Basic from './basic.vue';
  import Advanced from './advanced.vue';
  import AI from './ai.vue';
  import { getSettingInfo, Settings } from '@/apis/setting';
  import PageHeader from '@/components/pageHeader/pageHeader.vue';
  import { onMounted, ref, provide } from 'vue';
  import type { Dayjs } from 'dayjs';
  import { useI18n } from 'vue-i18n';

  const { t } = useI18n({});

  const activeKey = ref('basic');

  const title = {
    title: t('setting.title'),
    subTitle: t('setting.desc'),
  };

  const config = ref({
    message: {},
    other: {
      limit: 0,
      export: false,
    },
  } as Settings);

  function normalizeArray<T>(value: unknown, fallback: T[] = []): T[] {
    return Array.isArray(value) ? (value as T[]) : fallback;
  }

  const normalizeSettings = (settings: Settings): Settings => {
    settings.other = settings.other || ({} as Settings['other']);
    settings.other.idc = normalizeArray(settings.other.idc);
    settings.other.production_audit_tags = normalizeArray(
      settings.other.production_audit_tags,
      ['production', 'pre-production'],
    );
    settings.other.query_expire = normalizeArray<Dayjs>(
      settings.other.query_expire,
    );
    settings.other.overdue = normalizeArray<Dayjs>(settings.other.overdue);
    return settings;
  };

  const currentPage = async () => {
    const { data } = await getSettingInfo();
    config.value = normalizeSettings(data.payload);
  };

  provide('config', config);

  onMounted(() => {
    currentPage();
  });
</script>
