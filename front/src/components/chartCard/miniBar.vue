<template>
  <div :id="props.containerId"></div>
</template>

<script lang="ts" setup>
  import { onMounted } from 'vue';
  import { Chart } from '@antv/g2';
  import { getTopContext } from '@/apis/dash';

  const props = defineProps<{
    containerId: string;
    color: string;
  }>();

  onMounted(async () => {
    const { data } = await getTopContext();
    // 实机验证发现：目标库刚建/无统计数据时 payload 为 null，直接 sort 抛未捕获异常
    if (!Array.isArray(data.payload)) return;
    data.payload.sort((a: any, b: any) => {
      return a.count - b.count;
    });
    const chart = new Chart({
      container: props.containerId,
      autoFit: true,
      height: 450,
    });
    // G2 v5:encode() 通道 + coordinate transform;v4 的 element-active 更名
    // elementHighlight;主题色落到 mark style,不再经 styleSheet
    chart.data(data.payload);
    chart.axis('y', false);
    chart.axis('x', { title: false });
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
  });
</script>

<style lang="less" scoped>
  @import 'chart.less';
</style>
