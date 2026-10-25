import * as monaco from 'monaco-editor';
import { ref } from 'vue';
import { MysqlKeywords } from '@/components/editor/keyword';
import { lightWord } from '@/store/module/highlight';
import { queryHighlight } from '@/apis/source';

const createSQLToken = (
  range: any,
  exact: lightWord[],
): monaco.languages.CompletionItem[] => {
  const token = [] as any;
  MysqlKeywords.forEach((item: string) => {
    token.push({
      label: `"${item}"`,
      kind: monaco.languages.CompletionItemKind.Keyword,
      insertText: `${item}`,
      range: range,
      detail: '关键字',
    });
  });

  exact.forEach((item) => {
    token.push({
      label: item.vl,
      kind: monaco.languages.CompletionItemKind.Field,
      insertText: item.vl,
      range: range,
      detail: item.meta,
    });
  });
  return token;
};

interface UseSqlCompletionOptions {
  /** monaco 语言 id(三处现状均为 'sql'),保留参数以表达差异 */
  language?: string;
  /** 补全触发字符 */
  triggerCharacters?: string[];
}

// SQL 补全:query.vue / apply/order.vue / advisor/index.vue 原本逐字复制
// 「queryHighlight 取词 + registerCompletionItemProvider 注册 + dispose」三件套,收敛到这里。
//
// register 每次先注销上一份 provider,保证任意时刻只有一个补全源生效
// (原 fetchFields 里手写的「先 dispose 再注册」语义)。
// is_fields=false 的调用(取库/表名等非字段级词条)会把结果缓存在 nonFields,
// 供后续字段级注册并入 suggestions。
export function useSqlCompletion(options: UseSqlCompletionOptions = {}) {
  const { language = 'sql', triggerCharacters = ['.'] } = options;

  let provider: monaco.IDisposable | null = null;

  const nonFields = ref<lightWord[]>([]);

  const register = async (
    source_id: string,
    key: string,
    is_fields: boolean,
    source_fields: lightWord[],
  ) => {
    const { data } = await queryHighlight(source_id, is_fields, key);
    provider?.dispose();
    provider = monaco.languages.registerCompletionItemProvider(language, {
      provideCompletionItems: (
        model,
        position,
      ): monaco.languages.ProviderResult<monaco.languages.CompletionList> => {
        const word = model.getWordUntilPosition(position);
        const range = {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: word.startColumn,
          endColumn: word.endColumn,
        };
        return {
          suggestions: createSQLToken(range, [
            ...data.payload,
            ...source_fields,
          ]),
        };
      },
      triggerCharacters,
    });
    if (!is_fields) nonFields.value = data.payload;
  };

  const dispose = () => {
    provider?.dispose();
    provider = null;
  };

  return { nonFields, register, dispose };
}

export { createSQLToken };
