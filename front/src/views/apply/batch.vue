<template>
  <PageHeader
    title="项目工单（批量）"
    sub-title="项目版本迭代场景：一次提交多条 SQL 明细（可跨数据源/库），每条明细生成独立子工单并各自走其数据源的审批流"
  ></PageHeader>
  <a-row :gutter="24" type="flex" justify="center">
    <a-col :span="24">
      <a-card title="提交项目工单">
        <!-- 标签放控件上方 + 一行多列：原来「标签列 4/24 + 控件列 16/24」左右各留一截空白，
            四个字段还竖向排成四行。现在短字段一行三个、说明独占一行 -->
        <a-form layout="vertical">
          <a-row :gutter="16">
            <a-col :xs="24" :sm="12" :xl="6">
              <a-form-item label="工单类型">
                <a-radio-group v-model:value="type">
                  <a-radio-button :value="1">DML</a-radio-button>
                  <a-radio-button :value="0">DDL</a-radio-button>
                </a-radio-group>
              </a-form-item>
            </a-col>
            <a-col :xs="24" :sm="12" :xl="6">
              <a-form-item label="是否备份">
                <a-radio-group v-model:value="backup">
                  <a-radio :value="1">是</a-radio>
                  <a-radio :value="0">否</a-radio>
                </a-radio-group>
              </a-form-item>
            </a-col>
            <a-col :xs="24" :sm="24" :xl="12">
              <!-- 12/24 留给执行方式：定时执行时的日期选择器要和单选组并排 -->
              <a-form-item label="执行方式">
                <a-radio-group v-model:value="execMode">
                  <a-radio value="now">立即执行</a-radio>
                  <a-radio value="schedule">定时执行</a-radio>
                  <a-radio value="manual">人工执行</a-radio>
                </a-radio-group>
                <a-date-picker
                  v-if="execMode === 'schedule'"
                  show-time
                  style="margin-left: 8px"
                  :disabled-date="disabledDate"
                  :disabled-time="disabledTime"
                  @ok="delayTime"
                />
              </a-form-item>
            </a-col>
            <a-col :span="24">
              <a-form-item label="工单说明">
                <a-textarea
                  v-model:value="text"
                  :rows="2"
                  placeholder="如：v2.3.0 版本迭代"
                  show-count
                  allow-clear
                ></a-textarea>
              </a-form-item>
            </a-col>
          </a-row>
        </a-form>
      </a-card>

      <!-- 明细卡片与提交按钮提到页面层级：原来嵌在「提交项目工单」卡片里，卡片套卡片显重 -->
      <a-card
        v-for="(b, i) in blocks"
        :key="b.uid"
        size="small"
        style="margin-top: 16px"
      >
        <template #title>
          <span>明细 {{ i + 1 }}</span>
        </template>
        <template #extra>
          <a-button
            v-if="blocks.length > 1"
            type="text"
            danger
            @click="removeBlock(i)"
            >删除</a-button
          >
        </template>
        <!-- 左：数据源/目标库/上传；右：SQL 编辑器。左栏只占 5/24（标签在控件上方，
             不需要横向标签列），省下的宽度都给编辑器；下拉宽度跟着左栏一起收窄。
             左栏内容竖向拉伸、上传按钮贴底，跟右侧编辑器+操作条的高度对齐，不在下方留空洞。
             xl 以下两栏各自占满整行（退化回上下排） -->
        <a-row :gutter="16">
          <a-col :xs="24" :xl="5" class="block-side">
            <a-form layout="vertical">
              <a-form-item :label="$t('common.table.source')" :required="true">
                <a-select
                  v-model:value="b.source_id"
                  class="block-select"
                  show-search
                  :options="sourceOptions"
                  placeholder="选择数据源"
                  @change="() => onSourceChange(b)"
                ></a-select>
              </a-form-item>
              <a-form-item :label="$t('common.table.schema')" :required="true">
                <a-select
                  v-model:value="b.data_base"
                  class="block-select"
                  show-search
                  placeholder="选择目标库"
                >
                  <a-select-option v-for="d in b.dbList" :key="d">{{
                    d
                  }}</a-select-option>
                </a-select>
              </a-form-item>
            </a-form>
            <a-space wrap class="block-upload">
              <a-upload
                :before-upload="(f: File) => loadBlockFile(f, b.uid)"
                :show-upload-list="false"
                accept=".sql,.txt"
              >
                <a-button size="small">上传SQL文件</a-button>
              </a-upload>
              <span v-if="b.file" style="color: #888; word-break: break-all">{{
                b.file
              }}</span>
            </a-space>
          </a-col>
          <a-col :xs="24" :xl="19">
            <Editor
              :ref="el => setEditorRef(el, b.uid)"
              :container-id="`batch-editor-${b.uid}`"
              @get-values="sql => checkBlock(b.uid, sql as string)"
            ></Editor>
            <a-alert
              v-if="b.errCount > 0"
              type="error"
              :message="`存在 ${b.errCount} 条错误级规则未通过，请修正后再提交`"
              style="margin-top: 8px"
            />
          </a-col>
        </a-row>
      </a-card>

      <!-- 页面级动作条：加明细在左、提交在右，明细增删时位置固定好找 -->
      <div class="batch-actions">
        <a-button @click="addBlock">+ 添加明细</a-button>
        <a-button
          type="primary"
          :loading="posting"
          :disabled="blocks.some(b => b.errCount > 0)"
          @click="postBatch"
          >提交项目工单</a-button
        >
      </div>

      <a-card title="批次进度 / 批量执行" style="margin-top: 16px">
        <a-space>
          <a-input
            v-model:value="batchId"
            placeholder="输入批次号查询进度"
            style="width: 280px"
            allow-clear
          />
          <a-button :loading="loadingBatch" @click="loadBatch">查询</a-button>
          <a-popconfirm
            title="按提交顺序串行执行批次内所有等待执行的子工单，任一失败即停止，确认执行？"
            @confirm="doExecute"
          >
            <a-button
              type="primary"
              danger
              :loading="executing"
              :disabled="batchRef.data.length === 0"
              >批量执行</a-button
            >
          </a-popconfirm>
        </a-space>

        <c-table
          v-if="batchRef.data.length"
          :tbl-ref="batchRef"
          style="margin-top: 16px"
          row-key="work_id"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'status'">
              <a-tag :color="statusMap[record.status]?.color">{{
                statusMap[record.status]?.txt ?? record.status
              }}</a-tag>
            </template>
          </template>
        </c-table>

        <a-list
          v-if="execResults.length"
          size="small"
          style="margin-top: 16px"
          :data-source="execResults"
          row-key="work_id"
        >
          <template #renderItem="{ item }">
            <a-list-item>
              <a-space>
                <a-tag :color="item.ok ? 'green' : 'red'">{{
                  item.ok ? '成功' : '失败'
                }}</a-tag>
                <span>{{ item.source }} / {{ item.data_base }}</span>
                <span style="color: #999">{{ item.work_id }}</span>
                <span v-if="item.msg">{{ item.msg }}</span>
              </a-space>
            </a-list-item>
          </template>
        </a-list>
      </a-card>
    </a-col>
  </a-row>
</template>

<script lang="ts" setup>
  // 页面文案暂未走 i18n（项目工单为新增页面，先中文落地）
  import { onMounted, reactive, ref, watch } from 'vue';
  import { tableRef } from '@/components/table';
  import { useRoute } from 'vue-router';
  import { message } from 'ant-design-vue';
  import dayjs, { Dayjs } from 'dayjs';
  import Editor from '@/components/editor/editor.vue';
  import PageHeader from '@/components/pageHeader/pageHeader.vue';
  import { querySourceList, querySchemaList } from '@/apis/source';
  import { readSQLFile } from '@/lib';
  import {
    userPostBatchOrder,
    getBatchOrders,
    executeBatchOrder,
    BatchExecuteResult,
    checkSQLS,
  } from '@/apis/orderPostApis';

  interface Block {
    // 稳定 id：列表 key / 编辑器 ref / container-id 都用它。
    // 原来用数组下标，删中间一条明细时 Vue 会就地复用组件（keys 0,1,2 → 0,1），
    // monaco 实例（onMounted 时按当时的 id 建好、之后不再重建）就跟着错位，SQL 会串台。
    uid: number;
    source_id: string;
    data_base: string;
    errCount: number;
    dbList: string[];
    file: string;
  }

  let blockSeq = 0;

  const newBlock = (): Block => ({
    uid: blockSeq++,
    source_id: '',
    data_base: '',
    errCount: 0,
    dbList: [],
    file: '',
  });

  const route = useRoute();

  const type = ref<number>(
    route.query.type === 'ddl' ? 0 : 1
  );

  const text = ref('');
  const backup = ref(0);
  const execMode = ref('now');
  const delay = ref('none');
  const posting = ref(false);
  const loadingBatch = ref(false);
  const executing = ref(false);

  const blocks = ref<Block[]>([newBlock()]);

  // 执行方式映射到后端 delay 字段：now='none'、manual='manual'、schedule=选完时间后填
  watch(
    execMode,
    v => {
      if (v === 'now') delay.value = 'none';
      else if (v === 'manual') delay.value = 'manual';
      else delay.value = '';
    },
    { immediate: true }
  );

  const editorRefs = ref<Record<number, any>>({});
  const setEditorRef = (el: any, uid: number) => {
    if (el) editorRefs.value[uid] = el;
  };

  const findBlock = (uid: number) => blocks.value.find(b => b.uid === uid);

  const sourceOptions = ref<{ label: string; value: string }[]>([]);

  const statusMap: Record<number, { txt: string; color: string }> = {
    0: { txt: '已驳回', color: 'red' },
    1: { txt: '执行成功', color: 'green' },
    2: { txt: '审核中', color: 'blue' },
    3: { txt: '等待定时执行', color: 'orange' },
    4: { txt: '执行失败', color: 'red' },
    5: { txt: '等待执行', color: 'orange' },
    6: { txt: '已撤回', color: 'default' },
  };

  const batchColumns = [
    { title: '工单号', dataIndex: 'work_id', key: 'work_id' },
    { title: '数据源', dataIndex: 'source', key: 'source' },
    { title: '目标库', dataIndex: 'data_base', key: 'data_base' },
    { title: '状态', key: 'status' },
    { title: '提交时间', dataIndex: 'date', key: 'date' },
    { title: '执行时间', dataIndex: 'execute_time', key: 'execute_time' },
  ];

  // 批次子工单列表（项目工单页：可拖列宽、截断内容悬停看全文）；不分页
  const batchRef = reactive<tableRef>({
    col: batchColumns as any,
    data: [] as any[],
    pageCount: 0,
    hidePagination: true,
    resizable: true,
  });
  const execResults = ref<BatchExecuteResult[]>([]);
  const batchId = ref('');

  const fetchSources = async () => {
    const { data } = await querySourceList(type.value === 1 ? 'dml' : 'ddl');
    sourceOptions.value = (data.payload as any[]).map(i => ({
      label: `${i.source}（${i.idc}）`,
      value: i.source_id,
    }));
  };

  const onSourceChange = async (b: Block) => {
    b.data_base = '';
    if (!b.source_id) return;
    const { data } = await querySchemaList(b.source_id, true);
    b.dbList = data.payload as string[];
  };

  // 上传 SQL 文件：浏览器端读入该条明细的编辑器，文件名随子工单保存。
  // 类型/大小/是否文本的校验统一在 readSQLFile 内完成
  const loadBlockFile = async (file: File, uid: number) => {
    const b = findBlock(uid);
    if (!b) return false;
    try {
      const sql = await readSQLFile(file);
      editorRefs.value[uid]?.ChangeEditorText(sql);
      b.file = file.name;
      if (b.source_id && b.data_base) checkBlock(uid, sql);
    } catch (e) {
      message.error((e as Error).message);
    }
    return false;
  };

  const addBlock = () => blocks.value.push(newBlock());

  const removeBlock = (i: number) => blocks.value.splice(i, 1);

  // 与单工单一致：错误级规则（level===1）阻断提交
  const checkBlock = async (uid: number, sql: string) => {
    const b = findBlock(uid);
    if (!b || !sql || sql.replace(/\s/g, '').length === 0) return;
    const { data } = await checkSQLS({
      source_id: b.source_id,
      data_base: b.data_base,
      kind: type.value,
      sql: sql,
    });
    b.errCount = (data.payload || []).filter(
      (x: any) => x.level === 1
    ).length;
  };

  const postBatch = async () => {
    if (blocks.value.some(b => !b.source_id || !b.data_base)) {
      message.error('每条明细都需要选择数据源与目标库');
      return;
    }
    const items = blocks.value.map(b => ({
      source_id: b.source_id,
      data_base: b.data_base,
      file: b.file,
      sql: editorRefs.value[b.uid]?.GetValue() || '',
    }));
    if (items.some(it => it.sql.trim() === '')) {
      message.error('SQL 内容不能为空');
      return;
    }
    posting.value = true;
    try {
      const { data } = await userPostBatchOrder({
        type: type.value,
        backup: backup.value,
        delay: delay.value,
        text: text.value,
        items,
      });
      message.success(`提交成功，批次号：${data.payload}`);
      batchId.value = data.payload as string;
      await loadBatch();
    } finally {
      posting.value = false;
    }
  };

  const loadBatch = async () => {
    if (!batchId.value) return;
    loadingBatch.value = true;
    try {
      const { data } = await getBatchOrders(batchId.value);
      batchRef.data = data.payload || [];
      execResults.value = [];
    } finally {
      loadingBatch.value = false;
    }
  };

  const doExecute = async () => {
    executing.value = true;
    try {
      const { data } = await executeBatchOrder(batchId.value);
      execResults.value = data.payload || [];
      await loadBatch();
    } finally {
      executing.value = false;
    }
  };

  const disabledDate = (current: Dayjs) =>
    current && current.valueOf() < dayjs().startOf('day').valueOf();

  const disabledTime = (current: Dayjs) => {
    const now = dayjs();
    if (current && current.isSame(now, 'day')) {
      const range = (start: number, end: number) => {
        const r = [] as number[];
        for (let i = start; i < end; i++) r.push(i);
        return r;
      };
      return {
        disabledHours: () => range(0, now.hour()),
        disabledMinutes: (hour: number) =>
          hour === now.hour() ? range(0, now.minute()) : [],
      };
    }
    return {};
  };

  const delayTime = (date: Dayjs) => {
    delay.value = date.format('YYYY-MM-DD HH:mm');
  };

  onMounted(fetchSources);
</script>

<style scoped>
/* 与「工单填写」左栏同样的处理：antd 默认表单项下间距 24px，而这里行高只有 32px，
   间距快赶上行高本身。压到 10px（本页控件区够宽，单选组本来就是一行，不需要再收紧） */
:deep(.ant-form-item) {
  margin-bottom: 10px;
}

/* 明细左栏：a-row 是 flex + align-items:stretch，列本身拉满行高。
   把列改成纵向 flex 后，上传按钮用 margin-top:auto 贴到底部，
   左栏就不会在右边编辑器下方留出一块空白 */
.block-side {
  display: flex;
  flex-direction: column;
}

/* 明细里的数据源/目标库下拉：宽度跟着左栏走（xl 下约 278px），
   但窄屏（<xl）左栏会整行铺满，没有这个上限下拉又会被拉到 800px+，
   所以封在 280px —— 与查询页（200px）、工单填写页（187px）同类下拉同一量级 */
.block-select {
  width: 100%;
  max-width: 280px;
}
.block-upload {
  margin-top: auto;
}

/* 页面级动作条：与上方卡片同间距 */
.batch-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 16px;
}
</style>
