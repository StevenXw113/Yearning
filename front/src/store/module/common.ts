import { defineStore } from 'pinia';
import { getFlowList, RespTPLs } from '@/apis/flow';
import { Res } from '@/config/request';
import { UseWebSocketReturn } from '@vueuse/core';
import { AxiosResponse } from 'axios';

export interface commonStore {
  idc: string[];
  flow: RespTPLs[];
  queryInfo: QueryInfo;
  schemaList: string[];
  spinning: boolean;
  sql: string;
  sock: any;
  schema: string;
  principal: any[];
}

export interface QueryInfo {
  source_id: string;
  source: string;
  schema: string;
}

// pinia 迁移（原 vuex common 模块）：不持久化
export const useCommonStore = defineStore('common', {
  state: (): commonStore => ({
    idc: [] as string[],
    flow: [] as RespTPLs[],
    queryInfo: {} as QueryInfo,
    schemaList: [],
    spinning: false,
    sql: '',
    sock: {} as UseWebSocketReturn<any>,
    schema: '',
    principal: [],
  }),
  actions: {
    GET_IDC(idc: string[]) {
      this.idc = idc;
    },
    GET_FLOWS() {
      getFlowList().then(
        (res: AxiosResponse<Res<RespTPLs[]>>) => (this.flow = res.data.payload)
      );
    },
    SET_SOURCE_SCHEMA(query: QueryInfo) {
      this.queryInfo = query;
    },
    SET_SCHEMA_List(f: { schema: string[]; source: string; source_id: string }) {
      this.schemaList = f.schema;
      this.queryInfo.source = f.source;
      this.queryInfo.source_id = f.source_id;
    },
    SET_SCHEMA(v: string) {
      this.schema = v;
    },
    SET_SPINNING() {
      this.spinning = !this.spinning;
    },
    SET_DISABLED_SPINNING() {
      this.spinning = false;
    },
    ORDER_SET_SQL(vl: string) {
      this.sql = vl;
    },
    QUERY_CONN(vl: UseWebSocketReturn<any>) {
      this.sock = vl;
    },
    DB_SET_PRINCIPAL(vl: any[]) {
      this.principal = vl;
    },
  },
});
