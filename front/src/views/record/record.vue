<template>
  <a-page-header
    :title="title.title"
    :ghost="false"
    :sub-title="title.subTitle"
  >
    <div id="app-container">
      <div id="g2-customize-tooltip"></div>
      <div id="g2-container"></div>
    </div>

    <template #footer>
      <a-tabs v-model:activeKey="current" size="small">
        <a-tab-pane key="order" :tab="$t('common.order')"
          ><RecordOrder
        /></a-tab-pane>
        <a-tab-pane key="query" :tab="$t('common.query')"
          ><RecordQuery
        /></a-tab-pane>
        <a-tab-pane key="archive" :tab="$t('record.archive.tab')"
          ><RecordArchive
        /></a-tab-pane>
      </a-tabs>
    </template>
  </a-page-header>

  <a-back-top />
</template>

<script lang="ts" setup>
  import { getRecordAxis } from '@/apis/record';
  import { Chart } from '@antv/g2';
  import { onMounted, ref } from 'vue';
  import { useI18n } from 'vue-i18n';
  import RecordOrder from './order.vue';
  import RecordQuery from './query.vue';
  import RecordArchive from './archive.vue';

  const { t } = useI18n();
  const title = {
    title: t('record.title'),
    subTitle: '',
  };

  const current = ref<string>('order');

  const orderTypeChart = (id: string, data: any) => {
    const chart = new Chart({
      container: 'g2-container',
      autoFit: true,
      height: 300,
    });

    // G2 v5:type 的 0/1 → DDL/DML 直接在数据侧转换(替代 v4 的 scale.formatter,
    // v5 的 scale 键语义按通道而非字段);自定义 tooltip 用 interaction 原生 render
    const rows = (Array.isArray(data) ? data : []).map((i: any) => ({
      time: i.time,
      type: i.type === '0' ? 'DDL' : 'DML',
      count: i.count,
    }));
    chart.data(rows);
    chart.scale('y', { nice: true });
    chart.axis('y', {
      labelFormatter: (val: any) => `${val} /per`,
      grid: false,
    });

    chart
      .line()
      .encode('x', 'time')
      .encode('y', 'count')
      .encode('color', 'type')
      .encode('shape', 'smooth');

    chart
      .point()
      .encode('x', 'time')
      .encode('y', 'count')
      .encode('color', 'type')
      .encode('shape', 'circle');

    // customize tooltip
    const $tooltip = document.getElementById(
      'g2-customize-tooltip',
    ) as HTMLElement;

    function getTooltipHTML(data: { title: string; items: any }) {
      const { title, items } = data;
      return `
                  <div class="tooltip-title">${title}</div>
                  <div class="tooltip-items">
                  ${items
                    .map(
                      (datum: {
                        color: string;
                        name: string;
                        value: string;
                      }) => {
                        const color = datum.color;
                        const name = datum.name;
                        const value = datum.value;

                        return `
                        <div class="tooltip-item" style="border-left: 2px solid ${color}">
                        <div class="tooltip-item-name">${name}</div>
                        <div class="tooltip-item-value">${value} /per</div>
                       
                        </div>
        `;
                      },
                    )
                    .join('')}
    </div>
  `;
    }

    chart.on('afterrender', () => {
      // 获取最新值的数据

      const items = [
        {
          color: '#5B8FF9',
          name: 'DML',
          value: '0',
        },
      ];

      const data = {
        title: '0000-00-00',
        items,
      };

      $tooltip.innerHTML = getTooltipHTML(data);
    });

    // tooltip 的更新:v5 interaction 原生 render 取代 v4 的 tooltip:change 事件
    chart.interaction('tooltip', {
      crosshairs: true,
      render: (_evt: any, d: { title: string; items: any }) =>
        getTooltipHTML(d),
    });

    chart.render();
  };

  onMounted(async () => {
    const { data } = await getRecordAxis();
    orderTypeChart('container', data.payload);
  });
</script>

<!-- 原 insertCss 运行时注入的全局样式（G2 图表容器 + 自定义 tooltip），
     改为随组件打包的非 scoped 样式块；按 id 选择器命中，全局生效 -->
<style>
  #app-container {
    display: flex;
    flex-direction: column;
    width: 100%;
    height: 100%;
  }

  #g2-container {
    flex: auto;
  }

  #g2-customize-tooltip {
    margin-bottom: 16px;
    font-size: 12px;
  }

  #g2-customize-tooltip .tooltip-items {
    display: flex;
    flex-direction: row;
  }

  #g2-customize-tooltip .tooltip-item {
    flex-basis: 240px;
    padding-left: 8px;
    margin-right: 12px;
  }

  #g2-customize-tooltip .tooltip-item .tooltip-item-value {
    font-size: 16px;
    font-weight: bold;
  }

  #g2-customize-tooltip .tooltip-item-info {
    display: flex;
    justify-content: space-between;
  }

  #g2-customize-tooltip .tooltip-item-info .info-item {
    display: flex;
  }

  #g2-customize-tooltip .tooltip-item-info .info-item .info-item-name {
    opacity: 0.65;
  }

  #g2-customize-tooltip .tooltip-item-info .info-item .info-item-value {
    margin-left: 8px;
  }
</style>
