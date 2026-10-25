<template>
  <div class="antv-chart-mini">
    <div class="chart-wrapper">
      <div :id="props.containerId"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
  import { Chart } from '@antv/g2';
  const props = defineProps<{
    containerId: string;
    color: string;
    type: string;
  }>();

  const Randomdata = (total: any[]) => {
    let data = [] as { [key: string]: any }[];
    for (let i of total) {
      data.push({
        x: i.date,
        totals: props.type === 'query' ? i.total_query : i.total_order,
      });
    }
    return data;
  };

  // G2 v5:语法从 v4 链式 position() 改为 encode() 通道;主题色直接落到
  // mark 的 style 上(不再经 styleSheet.brandColor)
  const makeBuild = (total: any[]) => {
    const chart = new Chart({
      container: props.containerId,
      autoFit: true,
      height: 46,
    });
    chart.axis(false);
    chart.line()
      .data(Randomdata(total))
      .encode('x', 'x')
      .encode('y', 'totals')
      .encode('shape', 'smooth')
      .style('stroke', props.color);
    chart.area()
      .data(Randomdata(total))
      .encode('x', 'x')
      .encode('y', 'totals')
      .style('fill', props.color)
      .style('fillOpacity', 0.25);
    chart.interaction('tooltip', { crosshairs: true });
    chart.render();
  };

  defineExpose({
    makeBuild,
  });
</script>

<style lang="less" scoped>
  @import './chart.less';
</style>
