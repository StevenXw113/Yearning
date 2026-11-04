<template>
  <div class="antv-chart-mini">
    <div v-show="!empty" class="chart-wrapper">
      <div :id="props.containerId"></div>
    </div>
    <div v-if="empty" class="chart-empty">{{ $t('common.no.data') }}</div>
  </div>
</template>

<script setup lang="ts">
  import { nextTick, onBeforeUnmount, ref, watch } from 'vue';
  import { isDark } from '@/style/theme';
  import { g2Theme } from '@/lib/chartTheme';

  const props = defineProps<{
    containerId: string;
    color: string;
    type: string;
  }>();

  const empty = ref(true);

  let chart: import('@antv/g2').Chart | null = null;
  let rows: { x: string; totals: number }[] = [];

  const Randomdata = (total: any[]) => {
    // 库里还没有日汇总数据时 total_order 可能是 undefined（接口 payload 为 null），
    // 直接遍历会抛 "is not iterable"，把同级另一张图也一起带崩
    if (!Array.isArray(total)) return [];
    return total.map((i) => ({
      x: i.date,
      totals: props.type === 'query' ? i.total_query : i.total_order,
    }));
  };

  // G2 v5:语法从 v4 链式 position() 改为 encode() 通道;主题色直接落到
  // mark 的 style 上(不再经 styleSheet.brandColor)
  const build = async () => {
    // G2 1.3MB 动态加载:迷你图场景不值得常驻
    const { Chart } = await import('@antv/g2');
    // 主题切换/重复绘制前必须销毁旧实例，否则同一个容器里会叠出多张画布
    chart?.destroy();
    chart = new Chart({
      container: props.containerId,
      autoFit: true,
      height: 46,
      theme: g2Theme(),
    });
    chart.axis(false);
    chart
      .line()
      .data(rows)
      .encode('x', 'x')
      .encode('y', 'totals')
      .encode('shape', 'smooth')
      .style('stroke', props.color);
    chart
      .area()
      .data(rows)
      .encode('x', 'x')
      .encode('y', 'totals')
      .style('fill', props.color)
      .style('fillOpacity', 0.25);
    chart.interaction('tooltip', { crosshairs: true });
    chart.render();
  };

  const makeBuild = async (total: any[]) => {
    rows = Randomdata(total);
    empty.value = rows.length === 0;
    chart?.destroy();
    chart = null;
    if (empty.value) return;
    // v-show 刚放开，等容器拿到宽度再 autoFit，否则画布宽度是 0
    await nextTick();
    await build();
  };

  // Canvas 是位图，不会跟着 CSS 变量变色，主题切换后按新主题重建
  watch(isDark, () => {
    if (!empty.value) build();
  });

  onBeforeUnmount(() => chart?.destroy());

  defineExpose({
    makeBuild,
  });
</script>

<style lang="less" scoped>
  @import './chart.less';

  .chart-empty {
    position: absolute;
    bottom: 0;
    left: 0;
    width: 100%;
  }
</style>
