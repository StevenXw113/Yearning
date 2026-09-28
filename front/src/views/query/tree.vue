<template>
  <a-input-search
    v-model:value="searchValue"
    style="margin-bottom: 8px"
    :placeholder="$t('common.search')"
  />
  <a-spin :spinning="spinning">
    <a-tree
      v-model:expandedKey="expandedKeys"
      :auto-expand-parent="autoExpandParent"
      :tree-data="gData"
      :height="props.height"
      style="overflow: auto"
      show-icon
      @expand="onLoadData"
    >
      <template #switcherIcon="{ dataRef }">
        <hdd-outlined v-if="dataRef.meta === 'Schema'" />
      </template>
      <template #title="{ title, meta, key: treeKey }">
        <a-dropdown :trigger="['contextmenu']">
          <template v-if="title !== undefined">
            <span v-if="title.indexOf(searchValue) > -1">
              {{ title.substr(0, title.indexOf(searchValue)) }}
              <span style="color: #f50">{{ searchValue }}</span>
              {{
                title.substr(title.indexOf(searchValue) + searchValue.length)
              }}
            </span>
            <span v-else>{{ title }}</span>
          </template>
          <template #overlay>
            <a-menu v-if="meta === 'Table'">
              <a-menu-item key="1" @click="showTableData(treeKey)">
                {{ $t('query.show.table') }}</a-menu-item
              >
              <a-menu-item key="2" @click="showTableArch(treeKey)">
                {{ $t('query.show.arch') }}</a-menu-item
              >
            </a-menu>
          </template>
        </a-dropdown>
      </template>
    </a-tree>
  </a-spin>
</template>

<script lang="ts" setup>
  import { onMounted, ref, watch } from 'vue';
  import { querySchemaList, queryTable } from '@/apis/query';
  import { onBeforeRouteUpdate, useRoute } from 'vue-router';
  import { HddOutlined } from '@ant-design/icons-vue';
  import { useStore } from '@/store';
  import { TreeNodeProps } from 'ant-design-vue/lib/vc-tree';

  // 树高由父级传入（原来写死 700，比右侧 SQL 卡片高出一大截）
  const props = withDefaults(defineProps<{ height?: number }>(), { height: 700 });

  const emit = defineEmits(['showTableRef']);

  const route = useRoute();

  const searchValue = ref<string>('');

  const store = useStore();

  const expandedKeys = ref<string[]>([]);

  const autoExpandParent = ref<boolean>(false);

  const spinning = ref(false);

  const gData = ref<TreeNodeProps[]>([]);

  const schema = ref('');

  watch(searchValue, (value) => {
    let expanded: string[] = [];
    gData.value.forEach((item: any) => {
      // children 可能是空数组（下钻失败或该库没有表），原来的 children[0].key 会抛
      if (item.children?.length && item.children[0].key !== undefined) {
        item.children.forEach((el: any) => {
          if (el.title?.indexOf(value) > -1) {
            if (expanded.indexOf(item.title) == -1) expanded.push(item.title);
          }
        });
      }
    });
    expandedKeys.value = expanded;
    searchValue.value = value;
    autoExpandParent.value = false;
  });

  const onLoadData = async (keys: string, { expanded, node }: any) => {
    if (expanded) {
      if (node.dataRef.meta === 'Table') {
        return;
      }
      // 展开某个库 = 选中它：右键「查看表数据/表结构」要把库名一起发下去，
      // 否则 schema 一直是空串，Mongo 命令没有库执行不了（报「非法传参」），
      // MySQL 也会因为没选库而查不到
      schema.value = node.dataRef.title;
      spin();
      try {
        const { data } = await queryTable(
          route.query.source_id as string,
          node.dataRef.title
        );
        // 同上：接口失败时 payload 为 null，取 .table 会抛，spinner 也关不掉
        const tables = data.payload?.table || [];
        gData.value.filter((item: any) => {
          if (item.key === node.dataRef.key) {
            item.children = tables;
          }
        });
      } finally {
        spin();
      }
    }
  };

  const spin = () => {
    spinning.value = !spinning.value;
  };

  // 当前数据源类型（来自树根节点的 db_type）：Mongo(2) 的右键语句是命令 JSON，key 就是集合名
  const dbType = ref(0);

  const showTableData = (key: string) => {
    emit('showTableRef', {
      source_id: store.state.common.queryInfo.source_id,
      schema: schema.value,
      sql:
        dbType.value === 2
          ? JSON.stringify({ find: key, limit: 20 })
          : `select * from ${key}`,
    });
  };

  const showTableArch = (key: string) => {
    emit('showTableRef', {
      source_id: store.state.common.queryInfo.source_id,
      schema: schema.value,
      sql:
        dbType.value === 2
          ? JSON.stringify({ listIndexes: key })
          : `SHOW COLUMNS FROM ${key}`,
    });
  };

  const initial = async (source_id: string) => {
    spin();
    try {
      const { data } = await querySchemaList(source_id);
      // 接口失败（如无数据源权限）时 payload 是 null：直接赋给 tree-data 会让 antd 的
      // vc-tree 在 toRaw(props.treeData).slice() 上崩（它只判 undefined，null 漏过去），
      // 下面读 .length 也会抛。原来 spinner 只在「成功且有数据」时才关，失败会一直转圈。
      const list = data.payload || [];
      gData.value = list;
      dbType.value = list.length > 0 ? (list[0].db_type ?? 0) : 0;
      if (list.length > 0) {
        store.commit('common/SET_SCHEMA_List', {
          schema: list.map((item: { key: string }) => item.key),
          source: route.query.source as string,
          source_id: route.query.source_id as string,
        });
        store.commit('common/SET_SCHEMA', '');
      }
    } finally {
      spin();
    }
  };

  onBeforeRouteUpdate((to) => {
    initial(to.query.source_id as string);
  });

  onMounted(() => {
    initial(route.query.source_id as string);
  });
</script>
