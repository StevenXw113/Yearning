<template>
  <div>
    <div :id="props.containerId"></div>
    <div v-if="loaded && empty" class="chart-empty">
      {{ $t('common.no.data') }}
    </div>
  </div>
</template>

<script lang="ts" setup>
  import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
  import { getTopContext } from '@/apis/dash';
  import { isDark } from '@/style/theme';
  import { g2LabelFill, g2Theme } from '@/lib/chartTheme';

  const props = defineProps<{
    containerId: string;
    color: string;
  }>();

  // 无统计数据时画布只会剩一圈空坐标轴，改为直接给文案
  const empty = ref(true);
  // 请求返回前不要闪一下「暂无数据」
  const loaded = ref(false);

  let chart: import('@antv/g2').Chart | null = null;
  let rows: any[] = [];

  const build = async () => {
    // G2 1.3MB 动态加载：首页只有这一张柱图，不值得常驻
    const { Chart } = await import('@antv/g2');
    // 主题切换/重复绘制前必须销毁旧实例，否则同一个容器里会叠出多张画布
    chart?.destroy();
    chart = new Chart({
      container: props.containerId,
      autoFit: true,
      height: 450,
      theme: g2Theme(),
    });
    // G2 v5:encode() 通道 + coordinate transform;v4 的 element-active 更名
    // elementHighlight;主题色落到 mark style,不再经 styleSheet
    chart.data(rows);
    chart.axis('y', false);
    chart.axis('x', { title: false, labelFill: g2LabelFill() });
    chart.legend(false);
    chart.coordinate({ transform: [{ type: 'transpose' }] });
    chart
      .interval()
      .encode('x', 'source')
      .encode('y', 'count')
      .encode('size', 26)
      .style('fill', props.color);
    chart.interaction('elementHighlight');
    chart.render();
  };

  onMounted(async () => {
    const { data } = await getTopContext();
    // 实机验证发现：目标库刚建/无统计数据时 payload 为 null，直接 sort 抛未捕获异常
    rows = Array.isArray(data.payload) ? [...data.payload] : [];
    rows.sort((a: any, b: any) => a.count - b.count);
    empty.value = rows.length === 0;
    loaded.value = true;
    if (!empty.value) await build();
  });

  // Canvas 是位图，不会跟着 CSS 变量变色，主题切换后按新主题重建
  watch(isDark, () => {
    if (!empty.value) build();
  });

  onBeforeUnmount(() => chart?.destroy());
</script>

<style lang="less" scoped>
  @import 'chart.less';

  .chart-empty {
    padding: 24px 0;
  }
</style>
