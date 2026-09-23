<template>
  <PageHeader
    :title="$t('order.apply.commit.title')"
    :sub-title="$t('order.apply.commit.desc')"
  ></PageHeader>
  <a-row :gutter="24" type="flex" justify="center">
    <a-col :md="24" :xl="6">
      <a-card>
        <!-- 左栏保持 6/24：试过收到 5/24 给编辑器让宽（1600 下编辑器 1015→1075px），
             但两条路都不行——横排标签在 ≤1440 窗口下标签列只剩 ~50px，
             「执行方式」「是否回滚」必被裁；竖排标签不裁字却把左卡从 507px 撑到 747px，
             1366×768 这类屏幕下「获取表结构/上传SQL文件/提交」会掉出首屏。
             标签余量靠 style 里收窄冒号间距补，见 .ant-form-item-label::after -->
        <a-form
          v-bind="layout"
          ref="formRef"
          :model="orderItems"
          :rules="rules"
        >
          <a-form-item :label="$t('common.table.type')">
            <span>{{ orderItems.type === 0 ? 'DDL' : 'DML' }}</span>
          </a-form-item>
          <a-form-item :label="$t('common.table.env')">
            <span>{{ orderItems.idc }}</span>
          </a-form-item>
          <a-form-item :label="$t('common.table.source')">
            <span>{{ orderItems.source }}</span>
          </a-form-item>
          <a-form-item :label="$t('common.table.schema')" name="data_base">
            <a-select
              v-model:value="orderItems.data_base"
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
              v-model:value="orderItems.table"
              :dropdown-match-select-width="false"
              show-search
            >
              <a-select-option v-for="i in orderProfileArch.table" :key="i"
                >{{ i }}
              </a-select-option>
            </a-select>
          </a-form-item>
          <a-form-item :label="$t('common.table.remark')" name="text">
            <a-textarea
              v-model:value="orderItems.text"
              :rows="3"
              show-count
              allow-clear
            >
            </a-textarea>
          </a-form-item>
          <a-form-item :label="$t('order.profile.exec')">
            <a-radio-group v-model:value="execMode" name="execMode">
              <a-radio value="now">{{ $t('order.exec.now') }}</a-radio>
              <a-radio value="schedule">{{ $t('order.exec.schedule') }}</a-radio>
              <a-radio value="manual">{{ $t('order.exec.manual') }}</a-radio>
            </a-radio-group>
            <a-date-picker
              v-if="execMode === 'schedule'"
              show-time
              style="margin-left: 12px"
              :disabled-date="disabledDate"
              :disabled-time="disabledTime"
              @ok="delayTime"
            />
          </a-form-item>
          <a-form-item :label="$t('order.profile.roll')">
            <a-radio-group v-model:value="orderItems.backup" name="radioGroup">
              <a-radio :value="1">{{ $t('common.yes') }}</a-radio>
              <a-radio :value="0">{{ $t('common.no') }}</a-radio>
            </a-radio-group>
          </a-form-item>
        </a-form>
        <!-- 动作区：左栏卡片只有 ~260px，三个按钮同排会挤出卡片，
             这里改成整宽动作栏——次要操作一排（放不下自动换行）、文件名一行、主操作整宽 -->
        <a-space direction="vertical" :size="8" style="width: 100%">
          <a-space wrap :size="8">
            <a-button
              size="small"
              :loading="loadingTblBtn"
              @click="fetchTableArch"
              >{{ $t('order.apply.table.info') }}</a-button
            >
            <a-upload
              :before-upload="loadSQLFile"
              :show-upload-list="false"
              accept=".sql,.txt"
            >
              <a-button size="small">{{ $t('order.apply.upload') }}</a-button>
            </a-upload>
          </a-space>
          <a-typography-text
            v-if="orderItems.file"
            type="secondary"
            :title="orderItems.file"
            style="
              display: block;
              width: 100%;
              overflow: hidden;
              text-overflow: ellipsis;
              white-space: nowrap;
            "
          >
            {{ orderItems.file }}
          </a-typography-text>
          <a-button
            type="primary"
            block
            :loading="loadingPostBtn"
            :disabled="enabled"
            @click="postOrder"
            >{{ $t('common.commit') }}</a-button
          >
        </a-space>
      </a-card>
    </a-col>
    <a-col :sm="24" :md="24" :xl="18">
      <a-card>
        <a-tabs v-model:activeKey="activeKey">
          <a-tab-pane :key="1" :tab="$t('order.apply.tab.sql')" force-render>
            <a-spin :spinning="spin" :delay="100">
              <div class="editor_border">
                <Editor
                  ref="editor"
                  container-id="apply"
                  @get-values="testResults"
                  @change-content="() => (!enabled ? (enabled = true) : null)"
                >
                </Editor>
              </div>
              <br />
              <c-table :tbl-ref="sqlRef" row-key="sql"></c-table>
            </a-spin>
          </a-tab-pane>
          <a-tab-pane :key="2" :tab="$t('order.apply.tab.table')" force-render>
            <!-- 列宽有上限：内容超长截断显示（悬停看全文），列多则整体左右滚动 -->
            <c-table :tbl-ref="archRef" row-key="field"></c-table>
          </a-tab-pane>
          <a-tab-pane :key="3" :tab="$t('order.apply.tab.index')">
            <c-table :tbl-ref="idxRef" row-key="IndexName">
              <template #bodyCell="{ column, text }">
                <template v-if="column.dataIndex === 'NonUnique'">{{
                  text === 0 ? $t('common.yes') : $t('common.no')
                }}</template>
              </template>
            </c-table>
          </a-tab-pane>
        </a-tabs>
        <br />
        <a-steps size="small" progress-dot>
          <a-step
            v-for="i in orderProfileArch.timeline"
            :key="i.desc"
            :title="i.desc"
            status="process"
          >
            <template #subTitle>
              <a-tooltip placement="top">
                <template #title>
                  <span
                    >{{ $t('common.relevant') }}:
                    {{ i.auditor.join(' ') }}</span
                  >
                </template>
                {{ checkStepState(i.type) }}
              </a-tooltip>
            </template>
            <!-- <template v-slot:description>{{ $t('common.relevant') }}: {{ i.auditor.join(' ') }}</template> -->
          </a-step>
        </a-steps>
      </a-card>
    </a-col>
  </a-row>
</template>

<script lang="ts" setup>
  import Editor from '@/components/editor/editor.vue';
  import JunoMixin from '@/mixins/juno';
  import { onMounted, reactive, ref, onUnmounted, watch } from 'vue';
  import { tableRef } from '@/components/table';
  import { useRoute, onBeforeRouteLeave } from 'vue-router';
  import { SQLTesting } from '@/types';
  import FetchMixins from '@/mixins/fetch';
  import PageHeader from '@/components/pageHeader/pageHeader.vue';
  import {
    querySchemaList,
    queryTableList,
    queryTableArch,
    queryTimeline,
    queryHighlight,
  } from '@/apis/source';
  import dayjs, { Dayjs } from 'dayjs';
  import { message, Modal } from 'ant-design-vue';
  import {
    checkSQLS,
    SQLTestParams,
    userPostOrder,
  } from '@/apis/orderPostApis';
  import CommonMixins from '@/mixins/common';
  import { readSQLFile } from '@/lib';
  import router from '@/router';
  import { useStore } from '@/store';
  import { useI18n } from 'vue-i18n';
  import { debounce } from 'lodash-es';
  import { createSQLToken } from '@/components/editor/impl';
  import * as monaco from 'monaco-editor';

  const { t } = useI18n();

  // 标签列 7/24：6/24 时在 1366 窗口下只有 ~55px，而「执行方式」+ 冒号要 ~64px，
  // 会被 antd 裁掉（原有问题）。7/24 让 1366 下也有 ~68px；控件区仍有 165px+，
  // 1600 窗口下 207px 够「执行方式」两行排布
  const layout = {
    labelCol: { span: 7 },
    wrapperCol: { span: 17 },
  };

  const loadingTblBtn = ref(false);

  const loadingPostBtn = ref(false);

  const activeKey = ref(1);

  const spin = ref(false);

  const formRef = ref();

  const route = useRoute();

  const store = useStore();

  const enabled = ref(true);

  const { checkStepState } = CommonMixins();

  let monaco_editor: any = null;

  const rules = {
    data_base: [
      { required: true, message: t('common.check.source'), trigger: 'change' },
    ],
    text: [
      { required: true, message: t('common.check.text'), trigger: 'blur' },
    ],
  };

  const { col, orderItems, tableArch, indexArch } = JunoMixin();

  // 三张表：SQL 检测结果 / 表结构 / 索引（工单填写页：可拖列宽、截断内容悬停看全文）
  const sqlRef = reactive<tableRef>({
    col: col as any,
    data: [] as SQLTesting[],
    pageCount: 0,
    defaultPageSize: 10,
    resizable: true,
  });
  // 表结构/索引详情：列头由接口真实返回动态生成，列宽有上限——超长内容截断显示（悬停看全文）
  const archRef = reactive<tableRef>({
    col: tableArch as any,
    data: [],
    pageCount: 0,
    defaultPageSize: 10,
    resizable: true,
  });
  const idxRef = reactive<tableRef>({
    col: indexArch as any,
    data: [],
    pageCount: 0,
    defaultPageSize: 10,
    resizable: true,
  });

  // 前端本地数据：总条数 = 数据条数（分页在表格内完成）
  const fillTable = (ref: any, rows: any[]) => {
    ref.data = rows;
    ref.pageCount = rows.length;
  };

  const { orderProfileArch, editor } = FetchMixins();

  const nonFields = ref([] as any[]);

  const disabledDate = (current: Dayjs) => {
    // Can not select days before today and today
    return current && current.valueOf() < dayjs().startOf('day').valueOf();
  };

  const disabledTime = (current: Dayjs) => {
    // 禁用当前时间之前的时间
    const now = dayjs();
    if (current && current.isSame(now, 'day')) {
      return {
        disabledHours: () => range(0, now.hour()),
        disabledMinutes: (hour: number) => {
          if (hour === now.hour()) {
            return range(0, now.add(10, 'minute').minute());
          }
          return [];
        },
        disabledSeconds: (hour: number, minute: number) => {
          if (hour === now.hour() && minute === now.minute()) {
            return range(0, now.second());
          }
          return [];
        },
      };
    }
    return {};
  };

  const range = (start: number, end: number) => {
    const result = [] as number[];
    for (let i = start; i < end; i++) {
      result.push(i);
    }
    return result;
  };

  // 执行方式（提交时决定）：now=审批通过即执行、schedule=定时执行、manual=人工执行。
  // 后端只认 orderItems.delay：'none' / 'YYYY-MM-DD HH:mm' / 'manual'
  const execMode = ref('now');

  watch(
    execMode,
    (v) => {
      if (v === 'now') orderItems.delay = 'none';
      else if (v === 'manual') orderItems.delay = 'manual';
      else orderItems.delay = ''; // 选完时间后再由 delayTime 填
    },
    { immediate: true }
  );

  const delayTime = (date: Dayjs) => {
    orderItems.delay = date.format('YYYY-MM-DD HH:mm');
  };

  const fetchTable = async (schema: string) => {
    const { data } = await queryTableList(orderItems.source_id, schema);
    orderProfileArch.table = data.payload;
    fetchFields();
  };

  // 列头按接口真实返回的字段动态生成（SHOW FULL FIELDS / SHOW INDEX），不写死列集合；
  // 已知键给中文标题，其余直接用键名
  const fieldTitles: Record<string, string> = {
    field: t('order.table.field'),
    type: t('order.table.type'),
    collation: '字符集',
    null: t('order.table.isnull'),
    key: '键',
    default: t('order.table.default'),
    extra: '额外信息',
    privileges: '权限',
    comment: t('order.table.extra'),
  };
  const indexTitles: Record<string, string> = {
    Table: '所属表',
    NonUnique: t('order.table.isunique'),
    IndexName: t('order.table.index'),
    Seq: '序号',
    ColumnName: t('order.table.field'),
    IndexType: '索引类型',
  };
  // 每列宽度上限：超长内容按列宽截断显示（省略号 + 悬停 title 看全文），
  // 也避免 privileges / comment 这类长内容把表格撑到无限宽
  const fieldWidths: Record<string, number> = {
    field: 160,
    type: 150,
    collation: 160,
    null: 90,
    key: 80,
    default: 130,
    extra: 130,
    privileges: 180,
    comment: 220,
  };
  const indexWidths: Record<string, number> = {
    Table: 140,
    NonUnique: 90,
    IndexName: 160,
    Seq: 70,
    ColumnName: 300,
    IndexType: 120,
  };
  const columnsFrom = (
    rows: any[],
    titles: Record<string, string>,
    widths: Record<string, number>
  ) =>
    Object.keys(rows[0] || {}).map((k) => ({
      title: titles[k] || k,
      dataIndex: k,
      width: widths[k] || 150,
    }));

  const fetchTableArch = async () => {
    loadingTblBtn.value = true;
    const { data } = await queryTableArch(orderItems)
      .then((res) => {
        return res;
      })
      .finally(() => {
        loadingTblBtn.value = false;
      });
    if (data.payload.rows?.length) {
      archRef.col = columnsFrom(
        data.payload.rows,
        fieldTitles,
        fieldWidths
      ) as any;
      fillTable(archRef, data.payload.rows);
    }
    if (data.payload.idx?.length) {
      idxRef.col = columnsFrom(
        data.payload.idx,
        indexTitles,
        indexWidths
      ) as any;
      fillTable(idxRef, data.payload.idx);
    }
    activeKey.value = 2;
    message.success(t('order.apply.table.info') + t('common.success'));
  };

  const testResults = debounce(async (sql: string) => {
    if (sql.replace(/(^s*)|(s*$)/g, '').length == 0) {
      return;
    }
    spin.value = !spin.value;
    const { data } = await checkSQLS({
      source_id: orderItems.source_id,
      kind: orderItems.type,
      data_base: orderItems.data_base,
      sql: sql,
    } as SQLTestParams);
    let counter = 0;
    fillTable(sqlRef, data.payload);
    sqlRef.data.forEach((item: SQLTesting) => {
      // 只有 level===1（错误级规则）才拦；警告(2)/观察(3) 仅供参考，不影响提交。
      if (item.level === 1) {
        counter++;
      }
    });

    enabled.value = counter !== 0;
    spin.value = !spin.value;
  }, 200);

  const postOrder = () => {
    loadingPostBtn.value = !loadingPostBtn.value;
    formRef.value
      .validate()
      .then(async () => {
        let wrapper = Object.assign({}, orderItems);
        wrapper.sql = editor.value.GetValue();
        orderProfileArch.timeline.forEach((item) => {
          wrapper.relevant = wrapper.relevant.concat(item.auditor);
        });
        await userPostOrder(wrapper);
        enabled.value = true;
      })
      .catch(() => {
        message.error(t('order.apply.form.commit'));
      })
      .finally(() => (loadingPostBtn.value = !loadingPostBtn.value));
  };

  // 上传 SQL 文件：在浏览器端直接读入编辑器（不落服务端、不新增上传接口），
  // 文件名随工单保存（core_sql_orders.file），便于审批时追溯 SQL 来源。
  // 类型/大小/是否文本的校验统一在 readSQLFile 内完成
  const loadSQLFile = async (file: File) => {
    try {
      const sql = await readSQLFile(file);
      editor.value.ChangeEditorText(sql);
      orderItems.file = file.name;
      enabled.value = true;
      // 载入后立即检测：选定目标库时才有检测意义，有错误级规则会直接禁用提交
      if (orderItems.data_base) testResults(sql);
    } catch (e) {
      message.error((e as Error).message);
    }
    return false; // 阻止组件自身上传（内容已进编辑器）
  };

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

  const fetchHighLight = async () => {
    registerCompletionItemProvider(
      orderItems.source_id,
      orderItems.source_id,
      false,
      []
    );
  };

  const fetchSchema = async () => {
    const { data } = await querySchemaList(orderItems.source_id, true);
    orderProfileArch.db = data.payload;
  };

  const fetchTimeline = async () => {
    const { data } = await queryTimeline(orderItems.source_id, '');
    if (data.code === 5555) {
      // 该数据源没配审核流程，本页提交不了工单。
      // 原来直接 router.go(-1)：刷新/直接打开链接时没有可回退的历史，会跳到上一个站点或乱跳，
      // 页面上也不给任何解释。改成提示原因 + 回可操作的工单申请页（用 replace 避免后退又回到这页）
      message.error(data.text);
      router.replace('/apply/list');
      return;
    }
    orderProfileArch.timeline = data.payload;
  };

  onMounted(() => {
    orderItems.type = parseInt(route.query.type as string);
    orderItems.idc = route.query.idc as string;
    orderItems.source = route.query.source as string;
    orderItems.source_id = route.query.source_id as string;

    fetchSchema();
    fetchTimeline();
    fetchHighLight();

    route.query.remark === 'true'
      ? editor.value.ChangeEditorText(store.state.common.sql)
      : null;

    window.onbeforeunload = function () {
      if (editor.value.GetValue() !== '') {
        return t('common.leave');
      }
    };
  });

  onBeforeRouteLeave((to, from, next) => {
    if (editor.value.GetValue() !== '') {
      Modal.warn({
        content: t('common.leave'),
        onOk: () => {
          next();
        },
        // 点「取消」= 不离开，直接终止这次导航（原来写的是 router.go(11)，
        // 那是让浏览器历史前进 11 条，会跳到完全不相干的页面）
        onCancel: () => {
          next(false);
        },
        okCancel: true,
      });
    } else {
      next();
    }
  });

  onUnmounted(() => {
    window.onbeforeunload = null;
    monaco_editor.dispose();
  });
</script>

<style scoped>
/* 左栏是 xl=6 的窄卡（1600 下实测 340px 宽），antd 默认每个表单项 24px 的下间距
   已经和行高(32px)差不多，整列显得很空。压到 10px，8 行省下约 110px 高度。 */
:deep(.ant-form-item) {
  margin-bottom: 10px;
}
/* 标签列 6/24 约 65px，「执行方式」+ 冒号约 64px 卡在边缘；
   antd 给冒号留了 8px 右间距，收到 3px（并收左间距）腾出余量，避免被裁 */
:deep(.ant-form-item-label > label)::after {
  margin: 0 3px 0 1px;
}
/* 执行方式/是否回滚的单选组：控件区 187px，默认每个选项各占一行（实测 3 行 66px）。
   去掉 antd 的 8px 右间距后，列间距 ≤8px 才能两两排下（实测 2 行 46px） */
:deep(.ant-radio-group) {
  display: flex;
  flex-wrap: wrap;
  gap: 2px 8px;
}
:deep(.ant-radio-group .ant-radio-wrapper) {
  margin-right: 0; /* 间距交给上面的 gap，避免和 antd 的 8px 叠加 */
}
</style>
