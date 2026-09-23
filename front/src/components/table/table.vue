<template>
  <a-table
    v-bind="restAttrs"
    ref="tableEl"
    :class="{ 'tbl-fixed': isResizable }"
    :columns="renderCols"
    :size="props.size"
    :data-source="props.tblRef.data"
    :loading="loading"
    :pagination="pagination"
    :bordered="props.bordered"
    :table-layout="isResizable ? 'fixed' : 'auto'"
    :scroll="tableScroll"
    expand-row-by-click
    @change="currentPage"
    @expand="fillCellTitles"
    @resize-column="handleResizeColumn"
  >
    <template v-for="(_, name) in $slots" #[name]="slotData">
      <slot :name="name" v-bind="slotData"></slot>
    </template>
  </a-table>
</template>

<script lang="ts">
  // 外部传给 c-table 的 scroll（如 {y:400} 做固定表头）属于透传属性，会覆盖内部算出的横向
  // 宽度，导致表格回落到 max-content、拖不动列。这里关掉自动透传，改为手动转发（restAttrs），
  // 并把外部 scroll 与内部 x 合并后传给 a-table。
  export default { inheritAttrs: false };
</script>

<script lang="ts" setup>
  import {
    computed,
    nextTick,
    onMounted,
    onUnmounted,
    ref,
    useAttrs,
    watch,
  } from 'vue';
  import { useI18n } from 'vue-i18n';
  import * as t from '@/components/table';

  // 注意：上面的 `t` 是类型命名空间，i18n 这里换个名字避免遮蔽
  const { t: translate } = useI18n();

  interface propsAttr {
    tblRef: t.tableRef;
    bordered?: boolean;
    size?: string;
    isAll?: boolean;
  }

  const props = withDefaults(defineProps<propsAttr>(), {
    size: 'default',
    bordered: true,
  });

  // 统一开启列宽拖拽：给每列补 resizable 与数值宽度（没写宽度的给默认值）。
  // 用副本交给 a-table，而不是直接改父组件的列对象——在 setup/渲染期间写父组件的
  // 响应式数据会触发渲染期更新，Vue 会报 Unhandled error during execution of setup function。
  const defaultColWidth = 150;
  const colWidth = (w: any) => {
    const n = Number(w); // 兼容 '120' 这类字符串写法
    return Number.isFinite(n) && n > 0 ? n : defaultColWidth;
  };
  // 只有显式声明 resizable: true 的表格（工单相关界面）才启用「固定列宽 + 列宽拖拽 +
  // 超长截断」；其余表格保持 antd 原样（自适应布局、内容换行、无手柄、不截断）。
  const isResizable = computed(() => props.tblRef.resizable === true);
  const renderCols = computed(() =>
    isResizable.value
      ? (props.tblRef.col as any[]).map((col, idx) => ({
          ...col,
          resizable: true,
          width: colWidth(col.width),
          __idx: idx, // 拖拽回写时用来找回父组件的原列
        }))
      : (props.tblRef.col as any[])
  );

  const handleResizeColumn = (w: number, col: any) => {
    // 写回父组件原始列定义：renderCols 依赖它，会自动重建并应用新宽度
    const origin = (props.tblRef.col as any[])[col?.__idx];
    if (origin) origin.width = w;
    else if (col) col.width = w;
    fillCellTitles(); // 放宽/收窄后截断状态会变，重新挂提示
  };

  const tableEl = ref<any>(null);

  // 内容被截断的单元格补上原生 title，鼠标悬停即可看到完整内容
  // （ant-design-vue 3.2 的 ellipsis 只做样式、不会自动加 title）。
  const fillCellTitles = () => {
    nextTick(() => {
      const root = tableEl.value?.$el as HTMLElement | undefined;
      root?.querySelectorAll('.ant-table-tbody td').forEach((el) => {
        const td = el as HTMLElement;
        // 只给真的溢出（被截断）的单元格挂 title，避免整表都是提示
        if (td.scrollWidth > td.clientWidth) {
          td.setAttribute('title', (td.textContent || '').trim());
        } else {
          td.removeAttribute('title');
        }
      });
    });
  };

  watch(() => props.tblRef.data, fillCellTitles);

  // 表格宽度 = 各列宽度之和。默认的 scroll.x='max-content' 配合单元格 nowrap 会让表格
  // 宽度按「内容」撑开，拖窄长内容的列时被内容顶回去（拖不动）；用固定数值才能 1:1 拖动。
  const scrollX = computed(() =>
    renderCols.value.reduce(
      (sum, col: any) => sum + (Number(col.width) || defaultColWidth),
      0
    )
  );

  const attrs = useAttrs();

  // 其余透传属性照常给 a-table（row-key、class 等），scroll 除外
  const restAttrs = computed(() => {
    const rest = { ...(attrs as any) };
    delete rest.scroll;
    return rest;
  });

  // 外部 scroll 与内部 x 合并：既保留固定表头/高度，又保住 1:1 的列宽拖拽。
  // 非拖拽表格只透传外部 scroll，横向宽度交给 antd 按容器自适应。
  const tableScroll = computed(() => {
    const outer = (attrs.scroll as any) || {};
    return isResizable.value ? { ...outer, x: scrollX.value } : outer;
  });

  let isloop: any;

  // 表格分页配置；hidePagination 的表格（详情抽屉等）关闭分页
  const pagination = computed<any>(() =>
    props.tblRef.hidePagination
      ? false
      : {
          total: props.tblRef.pageCount,
          showTotal: (total: number) =>
            translate('common.count', { count: total }),
          position: ['bottomLeft'],
          showSizeChanger: true,
          current: pNumber.value,
          defaultPageSize: props.tblRef.defaultPageSize,
        }
  );

  const loading = ref(true);

  const pSize = ref(10);

  const pNumber = ref(1);

  watch(props.tblRef, () => {
    !props.isAll ? (loading.value = false) : null;
  });

  // a-table 的 change 事件：分页与表头筛选一并交给业务侧处理（服务端筛选）
  const currentPage = (
    page: { current: number; pageSize: number },
    filters?: any
  ) => {
    pSize.value = page.pageSize;
    pNumber.value = page.current;
    props.tblRef.fn !== undefined
      ? props.tblRef.fn({
          expr: props.tblRef.expr,
          current: page.current,
          pageSize: page.pageSize,
          filters,
        })
      : null;
    !props.isAll && !props.tblRef.isloop ? (loading.value = true) : null;
  };

  // reset=true 专供「搜索」：从第 1 页开始查。
  // 沿用当前页码时，结果不足一页（如按编号只搜到 1 条却请求第 2 页）会显示「暂无数据」。
  const manual = (reset = false) => {
    if (reset) pNumber.value = 1;
    props.tblRef.fn({
      expr: props.tblRef.expr,
      current: pNumber.value,
      pageSize: pSize.value,
    });
  };

  const loop = () => {
    props.tblRef.isloop
      ? (isloop = setInterval(() => {
          props.tblRef.fn({
            expr: props.tblRef.expr,
            current: pNumber.value,
            pageSize: pSize.value,
          });
        }, 5000))
      : null;
  };

  onUnmounted(() => {
    props.tblRef.isloop ? clearInterval(isloop) : null;
  });

  onMounted(() => {
    props.isAll
      ? (loading.value = false)
      : (loading.value = props.tblRef.data.length === 0);
    nextTick(() => {
      props.tblRef.defaultPageSize !== undefined
        ? (pSize.value = props.tblRef.defaultPageSize as number)
        : 10;
      props.tblRef.fn !== undefined ? manual() : null;
    });
    // 数据在挂载前就绪的表格（如规则列表）不会触发 data 的 watch，这里补一次
    fillCellTitles();
    loop();
  });

  //   onActivated(() => loop());

  //   onDeactivated(() => {
  //     props.tblRef.isloop ? clearInterval(isloop) : null;
  //   });

  defineExpose({
    manual,
  });
</script>

<style scoped>
/* 仅工单相关表格（tblRef.resizable === true）：拖窄后内容溢出就截断（配省略号），
   而不是把列顶回去；表宽布局由 a-table 的 table-layout="fixed" 保证。
   注意类落在 a-table 的根节点 .ant-table-wrapper 上。 */
.ant-table-wrapper.tbl-fixed :deep(.ant-table-tbody > tr > td) {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* antd 会给表格内联 min-width:100%：列宽之和小于容器时表格被拉宽，多出的宽度会被
   摊到各列上，拖动又被抵消。这里取消拉伸，列宽严格等于所设值（表格右侧可能留白）。 */
.ant-table-wrapper.tbl-fixed :deep(.ant-table-content > table),
.ant-table-wrapper.tbl-fixed :deep(.ant-table-header > table),
.ant-table-wrapper.tbl-fixed :deep(.ant-table-body > table) {
  min-width: 0 !important;
}
</style>
