<template>
  <a-spin :spinning="spinning">
    <a-card ref="colsize">
      <a-row>
        <a-col :span="5">
          <a-space>
            <ArrowLeftOutlined @click="$router.go(-1)" />
            <a-button size="small" type="primary" @click="() => (hide = !hide)"
              >{{ $t('common.hide') }}/{{ $t('common.visible') }}</a-button
            >
            <a-button size="small" @click="m.turnState()"
              >{{ $t('common.new') }}{{ $t('common.clip') }}</a-button
            >
            <a-button v-if="!isOnly" size="small" @click="ss.turnState()">{{
              $t('query.query.switch')
            }}</a-button>
            <a-popconfirm :title="$t('query.query.end.tips')" @confirm="undo">
              <a-button size="small">{{ $t('query.query.end') }}</a-button>
            </a-popconfirm>
          </a-space>
        </a-col>
      </a-row>
    </a-card>
    <br />
    <a-row :gutter="[16, 16]" style="margin-top: 1%">
      <a-col :span="hide ? 0 : 4">
        <a-card ref="leftSize" size="small">
          <a-tabs v-model:activeKey="tool">
            <a-tab-pane key="tree" :tab="$t('common.table.schema')">
              <Tree :height="treeHeight" @show-table-ref="showTableRef"></Tree>
            </a-tab-pane>
            <a-tab-pane key="history" :tab="$t('common.history')">
              <History></History>
            </a-tab-pane>
          </a-tabs>
        </a-card>
      </a-col>
      <a-col :span="hide ? 24 : 20">
        <div :style="{ width: width - leftz - 15 + 'px' }">
          <a-card ref="rightSize" size="small">
            <a-tabs v-model:activeKey="feat">
              <a-tab-pane key="edit" :tab="$t('query.query')">
                <a-tabs
                  v-model:activeKey="activeKey"
                  type="editable-card"
                  @edit="onEdit"
                >
                  <a-tab-pane
                    v-for="pane in panes"
                    :key="pane.key"
                    :tab="pane.title"
                    :closable="pane.closable"
                  >
                    <div class="editor_border">
                      <Input :id="pane.title" @on-change="fetchFields" />
                    </div>
                  </a-tab-pane>
                </a-tabs>
              </a-tab-pane>
              <a-tab-pane key="table" :tab="$t('query.table')" force-render>
                <Table id="tblInfo" ref="tbl" :height="800"></Table>
              </a-tab-pane>
            </a-tabs>
          </a-card>
        </div>
      </a-col>
    </a-row>
  </a-spin>
  <Clip></Clip>
  <Modal ref="m"></Modal>
  <Switch ref="ss"></Switch>
</template>

<script lang="ts" setup>
  import { wsHeartbeat } from '@/socket';
  import History from './history.vue';
  import Tree from './tree.vue';
  import Input from './input.vue';
  import Table from './table.vue';
  import Clip from './clip.vue';
  import Modal from './modal.vue';
  import Switch from './switch.vue';
  import { computed, onMounted, onUnmounted, ref } from 'vue';
  import { useUserStore, useCommonStore } from '@/store';
  import { ArrowLeftOutlined } from '@ant-design/icons-vue';
  import { checkIsQuery, queryUndoOrder } from '@/apis/query';
  import router from '@/router';
  import { onBeforeRouteUpdate, useRoute } from 'vue-router';
  import { useSqlCompletion } from '@/components/editor/impl';
  import { useElementSize, useWebSocket } from '@vueuse/core';
  import { checkSchema } from '@/lib';
  import { COMMON_URI } from '@/config/request';
  import { App } from 'ant-design-vue';
  const { message } = App.useApp();
  import { useI18n } from 'vue-i18n';

  const panes = ref([{ title: 'Untitled 1', key: '1', closable: false }]);

  const activeKey = ref();

  const colsize = ref();

  const leftSize = ref();

  const leftz = useElementSize(leftSize).width;

  const { t } = useI18n();

  const { width } = useElementSize(colsize);

  const rightSize = ref();

  const rightHeight = useElementSize(rightSize).height;

  // 左栏那棵树的高度跟着右侧卡片走，两栏底边对齐。
  // 128 = 树上方占位（tabs 头 46 + 搜索框 40 + 卡片内边距 24 等），原值写死 700，
  // 比右侧 SQL 卡片高出一大截，左栏会比右栏长出一块。
  const treeHeight = computed(() =>
    Math.max(240, Math.round(rightHeight.value) - 128),
  );

  const hide = ref(false);

  const newTabIndex = ref(1);

  const feat = ref('edit');

  const tool = ref('tree');

  const tbl = ref();

  const isOnly = ref(true);

  const m = ref();

  const ss = ref();

  const userStore = useUserStore();
  const commonStore = useCommonStore();

  const route = useRoute();

  const {
    nonFields,
    register: registerCompletion,
    dispose: disposeCompletion,
  } = useSqlCompletion();

  const spinning = computed(() => commonStore.spinning);

  const onEdit = (targetKey: string, action: string) => {
    if (action === 'add') {
      add();
    } else {
      remove(targetKey);
    }
  };

  const showTableRef = (vl: any) => {
    tbl.value.runResults(vl.schema, vl.sql);
    feat.value = 'table';
  };

  const add = () => {
    activeKey.value = `Untitled${++newTabIndex.value}`;
    panes.value.push({
      title: activeKey.value,
      key: activeKey.value,
      closable: true,
    });
  };

  const remove = (targetKey: string) => {
    let lastIndex = 0;
    panes.value.forEach((pane, i) => {
      if (pane.key === targetKey) {
        lastIndex = i - 1;
      }
    });
    panes.value = panes.value.filter((pane) => pane.key !== targetKey);
    if (panes.value.length && activeKey.value === targetKey) {
      if (lastIndex >= 0) {
        activeKey.value = panes.value[lastIndex].key;
      } else {
        activeKey.value = panes.value[0].key;
      }
    }
  };

  const undo = async () => {
    await queryUndoOrder();
    router.go(-1);
  };

  const initQuery = (source_id: string) => {
    let sock = useWebSocket(
      `${checkSchema()}${COMMON_URI}/query/results?source_id=${source_id}`,
      {
        autoReconnect: {
          retries: 3,
          delay: 1000,
          onFailed() {
            message.error(t('query.ws.error'));
          },
        },
        onDisconnected: () => {
          commonStore.SET_DISABLED_SPINNING();
        },
        heartbeat: wsHeartbeat(),
        protocols: [userStore.account.token],
      },
    );

    commonStore.QUERY_CONN(sock);
  };

  // SQL 补全:字段级(带 schema)注册时并入 nonFields 里缓存的库/表名词条
  const fetchFields = () => {
    registerCompletion(
      route.query.source_id as string,
      commonStore.schema,
      true,
      nonFields.value,
    );
  };

  const initial = (source_id: string) => {
    registerCompletion(source_id, source_id, false, []);
  };

  onBeforeRouteUpdate((to) => {
    disposeCompletion();
    initial(to.query.source_id as string);
    initQuery(to.query.source_id as string);
  });

  onMounted(async () => {
    const { data } = await checkIsQuery();
    isOnly.value = data.payload.status;
    initQuery(route.query.source_id as string);
    initial(route.query.source_id as string);
  });

  onUnmounted(() => {
    disposeCompletion();
  });
</script>
