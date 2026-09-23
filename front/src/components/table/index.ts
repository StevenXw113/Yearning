import type { App } from 'vue';
import CTable from '@/components/table/table.vue';
import { TableColumnsType } from 'ant-design-vue';
import { WebSocketResult } from '@vueuse/core';

const components = [CTable];

export declare const install: (app: App) => App<any>;

export interface tableRef {
  col: TableColumnsType;
  data: any[];
  pageCount: number;
  expr?: any;
  fn?: any;
  defaultPageSize?: number;
  isloop?: boolean;
  websocket?: WebSocketResult<any>;
  // 表格自身不分页（如详情抽屉里的结果表 / 分页由外部组件负责）
  hidePagination?: boolean;
  // true：工单相关表格——固定列宽 + 可拖拽 + 超长截断（截断内容悬停看全文）
  // 不传/其它值：保持 antd 原样（自适应布局、内容换行、无拖拽手柄）——非工单界面不要开
  resizable?: boolean;
}

export default {
  install(app: App<any>) {
    components.forEach((comp) => {
      app.component('CTable', comp);
    });
  },
};
