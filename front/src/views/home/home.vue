<template>
  <div>
    <a-row :gutter="24">
      <a-col :xs="24" :md="12" :xl="6" :style="{ marginBottom: '24px' }">
        <ChartCard
          :loading="loading"
          :title="$t('common.order') + $t('common.sum')"
          :total="banner.order"
        >
          <template #action>
            <a-tooltip
              :title="$t('common.tip.order')"
              :trigger="['hover', 'click']"
            >
              <InfoCircleOutlined />
            </a-tooltip>
          </template>
          <template #content>
            <MiniArea
              ref="order"
              container-id="order"
              color="#2094F3"
              type="order"
            />
          </template>
        </ChartCard>
      </a-col>
      <a-col :xs="24" :md="12" :xl="6" :style="{ marginBottom: '24px' }">
        <ChartCard
          :loading="loading"
          :title="$t('common.query') + $t('common.sum')"
          :total="banner.query"
        >
          <template #action>
            <a-tooltip
              :title="$t('common.tip.query')"
              :trigger="['hover', 'click']"
            >
              <InfoCircleOutlined />
            </a-tooltip>
          </template>
          <template #content>
            <MiniArea
              ref="query"
              container-id="query"
              color="#Ff9900"
              type="query"
            />
          </template>
        </ChartCard>
      </a-col>
      <a-col :xs="24" :md="12" :xl="6" :style="{ marginBottom: '24px' }">
        <ChartCard
          :loading="loading"
          :title="$t('common.table.source') + $t('common.sum')"
          :total="banner.source"
        >
          <template #action>
            <a-tooltip
              :title="$t('common.tip.source')"
              :trigger="['hover', 'click']"
            >
              <InfoCircleOutlined />
            </a-tooltip>
          </template>
          <template #content>
            <a-progress
              :percent="58"
              status="active"
              :show-info="false"
              :stroke-width="15"
              :stroke-color="{
                '0%': '#108ee9',
                '100%': '#87d068',
              }"
            />
          </template>
        </ChartCard>
      </a-col>
      <a-col :xs="24" :md="12" :xl="6" :style="{ marginBottom: '24px' }">
        <ChartCard
          :loading="loading"
          :title="$t('menu.manage.user') + $t('common.sum')"
          :total="banner.user"
        >
          <template #action>
            <a-tooltip
              :title="$t('common.tip.user')"
              :trigger="['hover', 'click']"
            >
              <InfoCircleOutlined />
            </a-tooltip>
          </template>
          <template #content>
            <a-progress
              :percent="85"
              status="active"
              :show-info="false"
              :stroke-width="15"
              :stroke-color="{
                '0%': '#108ee9',
                '100%': '#87d068',
              }"
            />
          </template>
        </ChartCard>
      </a-col>
    </a-row>

    <a-card style="text-align: center; margin-bottom: 24px">
      <a-row :gutter="24">
        <a-col :xs="24" :md="12" :xl="6">
          <a-statistic
            :title="$t('common.bash.self.dml') + windowLabel"
            :value="banner.self_dml"
          />
        </a-col>
        <a-col :xs="24" :md="12" :xl="6">
          <a-statistic
            :title="$t('common.bash.self.ddl') + windowLabel"
            :value="banner.self_ddl"
          />
        </a-col>
        <a-col :xs="24" :md="12" :xl="6">
          <a-statistic
            :title="$t('common.bash.self.query') + windowLabel"
            :value="banner.self_query"
          />
        </a-col>
        <a-col :xs="24" :md="12" :xl="6">
          <a-statistic
            :title="$t('common.bash.self.audit') + windowLabel"
            :value="banner.self_audit"
          />
        </a-col>
      </a-row>
    </a-card>
    <a-row :gutter="24">
      <a-col :xs="24" :md="24" :xl="16" :style="{ marginBottom: '24px' }">
        <a-card :title="$t('common.board')">
          <div v-html="boardContent" />
        </a-card>
      </a-col>
      <a-col :xs="24" :md="24" :xl="8" :style="{ marginBottom: '24px' }">
        <a-card :title="$t('common.table.source') + ' Top10'">
          <MiniBar container-id="trend" color="#Ff9900" />
        </a-card>
      </a-col>
    </a-row>
  </div>
</template>

<script setup lang="ts">
  import ChartCard from '@/components/chartCard/chartCard.vue';
  import MiniArea from '@/components/chartCard/miniArea.vue';
  import MiniBar from '@/components/chartCard/miniBar.vue';
  import { InfoCircleOutlined } from '@ant-design/icons-vue';
  import { getBannerContext } from '@/apis/dash';
  import { getBoardContext } from '@/apis/board';
  import { sanitizeHTML } from '@/lib/sanitize';
  import { computed, nextTick, onMounted, ref } from 'vue';

  // 原来是 const loading = false（普通常量），ChartCard 的骨架屏永远不显示
  const loading = ref(true);

  const banner = ref<any>({
    total_order: [],
  });

  // 「我的」三项是后端按时间窗口统计的，标题带上窗口天数，避免和全量列表条数对不上时产生疑惑
  const windowLabel = computed(() =>
    banner.value.window_days ? ` / ${banner.value.window_days}d` : ''
  );

  const boardContent = ref<string>('');

  const query = ref();

  const order = ref();

  const getBanner = async () => {
    const { data } = await getBannerContext();
    banner.value = data.payload;
  };

  const getBoard = async () => {
    const { data } = await getBoardContext();
    // 公告内容来自服务端存储的富文本，渲染进 v-html 前必须白名单清洗，防止存储型 XSS
    boardContent.value = sanitizeHTML(data.payload);
  };

  onMounted(async () => {
    await Promise.all([getBanner(), getBoard()]);
    loading.value = false;
    // 骨架屏期间图表容器还没渲染，等它换成真实节点后再画
    await nextTick();
    // 日汇总表还没跑过定时任务时 total_order 可能为空/缺失，
    // 兜成数组让两张迷你图各自走「暂无数据」而不是抛错一起消失
    const series = Array.isArray(banner.value.total_order)
      ? banner.value.total_order
      : [];
    await Promise.all([
      order.value?.makeBuild(series),
      query.value?.makeBuild(series),
    ]);
  });
</script>
