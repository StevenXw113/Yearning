<template>
  <PageHeader
    :title="$t('menu.order.advisor')"
    :sub-title="$t('common.advisor.sub')"
  ></PageHeader>

  <a-card>
    <a-tabs v-model:activeKey="activeKey">
      <a-tab-pane :key="1" :tab="$t('order.apply.tab.sql')" force-render>
        <a-form
          ref="formRef"
          :model="orderItems"
          :rules="rules"
          layout="inline"
          style="width: 100%"
        >
          <a-form-item :label="$t('common.table.env')">
            <a-select
              v-model:value="orderItems.idc"
              :dropdown-match-select-width="false"
              show-search
              style="width: 150px"
              @change="fetchSource"
            >
              <a-select-option v-for="i in orderProfileArch.idc" :key="i"
                >{{ i }}
              </a-select-option>
            </a-select>
          </a-form-item>
          <a-form-item :label="$t('common.table.source')">
            <a-select
              v-model:value="orderItems.source_id"
              :dropdown-match-select-width="false"
              show-search
              style="width: 150px"
              @change="fetchSchema"
            >
              <a-select-option
                v-for="i in orderProfileArch.source"
                :key="i.source"
                :value="i.source_id"
                >{{ i.source }}
              </a-select-option>
            </a-select>
          </a-form-item>
          <a-form-item :label="$t('common.table.schema')" name="data_base">
            <a-select
              v-model:value="orderItems.data_base"
              style="width: 150px"
              :dropdown-match-select-width="false"
              show-search
              @change="fetchTable"
            >
              <a-select-option v-for="i in orderProfileArch.db" :key="i"
                >{{ i }}
              </a-select-option>
            </a-select>
          </a-form-item>
          <a-form-item :label="$t('common.table.table')">
            <a-select
              v-model:value="orderItems.tables"
              style="width: 150px"
              :dropdown-match-select-width="false"
              mode="multiple"
              show-search
            >
              <a-select-option v-for="i in orderProfileArch.table" :key="i"
                >{{ i }}
              </a-select-option>
            </a-select>
          </a-form-item>
        </a-form>
        <br />
        <div class="editor_border">
          <Editor
            ref="editor"
            is-advisor
            container-id="apply"
            @getSQLGen="text2sql"
            @get-values="advisor"
            @change-content="() => (!enabled ? (enabled = true) : null)"
          >
          </Editor>
        </div>
        <br />
        <a-card :title="$t('order.profile.results')" size="small">
          <a-spin :spinning="spin" :delay="100">
            <div
              id="previewResults"
              style="height: 300px; overflow: auto"
            ></div>
          </a-spin>
        </a-card>
      </a-tab-pane>
      <a-tab-pane :key="2" :tab="$t('order.apply.tab.assistant')" force-render>
        <iframe
          id="chat2"
          :src="`/chatbot?token=${store.state.user.account.token}`"
          style="width: 100%; height: 500px; border: none"
        />
      </a-tab-pane>
    </a-tabs>
  </a-card>
</template>

<script lang="ts" setup>
  // import Board from '@/components/board/index.vue';
  import Editor from '@/components/editor/editor.vue';
  import JunoMixin from '@/mixins/juno';
  import { onMounted, ref } from 'vue';
  import { useRoute } from 'vue-router';
  import FetchMixins from '@/mixins/fetch';
  import PageHeader from '@/components/pageHeader/pageHeader.vue';
  import {
    querySchemaList,
    queryTableList,
    queryHighlight,
    queryIDCList,
    querySourceList,
    ISource,
  } from '@/apis/source';
  import { useI18n } from 'vue-i18n';
  import { debounce } from 'lodash-es';
  import { createSQLToken } from '@/components/editor/impl';
  import * as monaco from 'monaco-editor';
  import { FetchSQLAdvisor } from '../../apis/advisor';
  import Vditor from 'vditor';
  import 'vditor/dist/index.css';
  import { useStore } from '@/store';

  const { t } = useI18n();

  const store = useStore();

  const activeKey = ref(1);

  const spin = ref(false);

  const formRef = ref();

  const route = useRoute();

  const enabled = ref(true);

  let monaco_editor: any = null;

  const rules = {
    data_base: [
      { required: true, message: t('common.check.source'), trigger: 'change' },
    ],
    text: [
      { required: true, message: t('common.check.text'), trigger: 'blur' },
    ],
  };

  const { orderItems } = JunoMixin();

  const { orderProfileArch, editor } = FetchMixins();

  const nonFields = ref([] as any[]);

  const fetch = async (type: string) => {
    const { data } = await FetchSQLAdvisor(orderItems, type);
    Vditor.preview(
      document.getElementById('previewResults') as any,
      data.payload
    );
  };

  const fetchTable = async (schema: string) => {
    const { data } = await queryTableList(orderItems.source_id, schema);
    orderProfileArch.table = data.payload;
    fetchFields();
  };
  const advisor = debounce(async (sql: string) => {
    orderItems.sql = sql;
    spin.value = true;
    await fetch('advisor');
    spin.value = false;
  }, 200);

  const text2sql = debounce(async (sql: string) => {
    orderItems.sql = sql;
    spin.value = true;
    await fetch('text2sql');
    spin.value = false;
  }, 200);

  const registerCompletionItemProvider = async (
    source_id: string,
    key: string,
    is_fields: boolean,
    source_fields: any[]
  ) => {
    const { data } = await queryHighlight(source_id, is_fields, key);
    monaco_editor = monaco.languages.registerCompletionItemProvider('sql', {
      provideCompletionItems: (
        model,
        position
      ): monaco.languages.ProviderResult<monaco.languages.CompletionList> => {
        let word = model.getWordUntilPosition(position);
        let range = {
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
      triggerCharacters: ['.'],
    });
    !is_fields ? (nonFields.value = data.payload) : null;
  };

  const fetchFields = async () => {
    monaco_editor !== null ? monaco_editor.dispose() : null;
    registerCompletionItemProvider(
      orderItems.source_id,
      orderItems.data_base,
      true,
      nonFields.value
    );
  };

  const fetchSchema = async () => {
    const { data } = await querySchemaList(orderItems.source_id, true);
    orderProfileArch.db = data.payload;
  };

  const fetchIDC = async () => {
    const { data } = await queryIDCList();
    orderProfileArch.idc = data.payload;
  };

  const fetchSource = async () => {
    const { data } = await querySourceList('query', orderItems.idc);
    orderProfileArch.source = data.payload as ISource[];
  };
  onMounted(() => {
    orderItems.type = 2;
    orderItems.idc = route.query.idc as string;
    orderItems.source = route.query.source as string;
    orderItems.source_id = route.query.source_id as string;
    fetchIDC();
  });
</script>
