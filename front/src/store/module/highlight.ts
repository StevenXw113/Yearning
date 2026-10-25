// SQL 补全词条(editor/impl.ts 的 createSQLToken/useSqlCompletion 消费)。
// 原 vuex/pinia highlight 模块已无调用方,只剩该接口被引用。
export interface lightWord {
  meta: string;
  vl: string;
}
