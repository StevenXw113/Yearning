import { COMMON_URI, request, Res } from '@/config/request';
import { Dayjs } from 'dayjs';

export interface Message {
  web_hook: string;
  key: string;
  host: string;
  ssl: boolean;
  port: number;
  user: string;
  password: string;
  to_user: string;
  mail: boolean;
  ding: boolean;
}

export interface LDAP {
  url: string;
  ldaps: boolean;
  user: string;
  password: string;
  type: string;
  sc: string;
  map: string;
  test_user: string;
  test_password: string;
}

export interface Other {
  limit: number;
  idc: string[];
  force: string;
  query: boolean;
  ex_query_time: number;
  query_expire: Dayjs[];
  overdue: Dayjs[];
  export: boolean;
  register: boolean;
  close_ai: string;
  proxy: string;
  domain: string;
}

export interface AI {
  base_url: string;
  api_key: string;
  frequency_penalty: number;
  max_tokens: number;
  presence_penalty: number;
  temperature: number;
  top_p: number;
  model: string;
  advisor_prompt: string;
  sql_gen_prompt: string;
  sql_agent_prompt: string;
  proxy_url: string;
}

export interface Settings {
  message: Message;
  ldap: LDAP;
  other: Other;
  ai: AI;
}

export interface DeleteOrder {
  date: string[];
  tp: boolean;
}

export function getSettingInfo() {
  return request.get<Res<Settings>>(`${COMMON_URI}/manage/setting`);
}

export function testMessageHook(tp: string, testArgs: Settings) {
  return request.put(`${COMMON_URI}/manage/setting?test=${tp}`, testArgs);
}

export function updateSettingInfo(params: Settings) {
  return request.post(`${COMMON_URI}/manage/setting`, params);
}

export function deleteOrderRecords(args: DeleteOrder) {
  return request.delete(`${COMMON_URI}/manage/setting`, {
    data: args,
  });
}
