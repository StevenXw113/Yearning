<template>
  <!-- 宽度对齐下方卡片第一列（同一个容器里，按列表栅格算宽度），见 style -->
  <div class="source-filter">
    <SearchOutlined class="source-filter-icon" />
    <a-select
      v-model:value="selected"
      show-search
      allow-clear
      placeholder="数据源搜索"
      style="width: 100%"
      :filter-option="filterOption"
      @change="handleChange"
    >
      <a-select-option value="all">{{
        $t('order.state.all')
      }}</a-select-option>
      <a-select-option
        v-for="i in options"
        :key="i.source"
        :value="i.source"
        >{{ i.source }}</a-select-option
      >
    </a-select>
  </div>
  <br />
  <a-list
    :loading="loading"
    :data-source="source"
    :grid="{ gutter: 16, xs: 1, sm: 1, md: 2, lg: 2, xl: 4, xxl: 4, xxxl: 4 }"
    :pagination="pagination"
  >
    <template #renderItem="{ item }">
      <a-list-item>
        <div
          @click="
            () =>
              router.push({
                path: props.type !== 'query' ? '/apply/order' : '/apply/query',
                query: {
                  type: props.id,
                  idc: item.idc,
                  source: item.source,
                  source_id: item.source_id,
                  db_type: item.db_type,
                },
              })
          "
        >
          <a-card :body-style="{ paddingBottom: 20 }" hoverable>
            <a-card-meta :title="item.source">
              <template #description>{{
                $t('order.apply.card.env', {
                  env: item.idc,
                })
              }}</template>
              <template #avatar>
                <a-avatar :style="{ backgroundColor: '#Ff9900' }">
                  <template #icon>
                    <CodepenCircleOutlined />
                  </template>
                </a-avatar>
              </template>
            </a-card-meta>
            <template #actions>
              <a-tooltip
                :title="
                  $t('order.apply.tab.source_id', { env: item.source_id })
                "
              >
                <SubnodeOutlined />
              </a-tooltip>
              <a-tooltip :title="$t('order.apply.card.env', { env: item.idc })">
                <ShareAltOutlined />
              </a-tooltip>
              <a-dropdown>
                <a-tooltip :title="$t('order.apply.card.enter')">
                  <a
                    class="ant-dropdown-link"
                    @click="
                      () =>
                        router.push({
                          path:
                            props.type !== 'query'
                              ? '/apply/order'
                              : '/apply/query',
                          query: {
                            type: props.id,
                            idc: item.idc,
                            source: item.source,
                            source_id: item.source_id,
                            db_type: item.db_type,
                          },
                        })
                    "
                  >
                    <EnterOutlined />
                  </a>
                </a-tooltip>
              </a-dropdown>
            </template>
          </a-card>
        </div>
      </a-list-item>
    </template>
  </a-list>
</template>

<script lang="ts" setup>
  import {
    SubnodeOutlined,
    EnterOutlined,
    ShareAltOutlined,
    CodepenCircleOutlined,
    SearchOutlined,
  } from '@ant-design/icons-vue';
  import { onMounted, ref } from 'vue';
  import { useRouter } from 'vue-router';
  import { ISource, querySourceList } from '@/apis/source';

  const props = defineProps<{
    type: string;
    id: number;
    isExport?: boolean;
  }>();

  const router = useRouter();

  const pagination = {
    pageSize: 20,
  };

  const filterOption = (input: string, option: any) => {
    return option.value.toLowerCase().indexOf(input.toLowerCase()) >= 0;
  };

  const handleChange = (value: string) => {
    value === '' || value === undefined || value === 'all'
      ? (source.value = tmpSource)
      : (source.value = tmpSource.filter(
          (item: ISource) => item.source === value
        ));
  };

  const selected = ref('all');

  let tmpSource = [] as ISource[];

  const source = ref([] as ISource[]);

  const options = ref([] as ISource[]);

  const loading = ref(true);

  onMounted(async () => {
    try {
      const { data } = await querySourceList(props.type);
      tmpSource = source.value = options.value = data.payload as ISource[];
      loading.value = false;
    } catch (error) {
      console.log(error);
    }
  });
</script>

<style scoped>
/* 数据源筛选框：左侧放大镜 + 圆角胶囊，颜色用中性透明度叠加，
   深/浅主题都不需要单独适配（强调色用主题的 primary #527590） */
.source-filter {
  position: relative;
  /* 宽度对齐下方 a-list 卡片的第一列，与列表栅格同一套断点：
     xs/sm 1 列、md/lg 2 列、xl+ 4 列，gutter 16。
     卡片宽 = (容器宽 + 16) / 列数 - 16，又因为 a-list 的 row 有 -8px 外边距，
     换算到容器百分比就是下面这三个值（1440 下 25% - 12px = 299px，与卡片一致）*/
  width: 100%;
}

@media (min-width: 768px) {
  .source-filter {
    width: calc(50% - 8px);
  }
}

@media (min-width: 1200px) {
  .source-filter {
    width: calc(25% - 12px);
  }
}

.source-filter-icon {
  position: absolute;
  top: 50%;
  left: 11px;
  z-index: 1;
  font-size: 13px;
  color: inherit;
  opacity: 0.4;
  pointer-events: none;
  transform: translateY(-50%);
  transition: opacity 0.2s;
}

.source-filter:focus-within .source-filter-icon {
  opacity: 1;
}

.source-filter :deep(.ant-select-single) {
  height: 36px;
}

.source-filter :deep(.ant-select-single .ant-select-selector) {
  height: 36px;
  padding-left: 30px;
  border-radius: 8px;
  background: rgba(128, 128, 128, 0.08);
  transition: background 0.2s, border-color 0.2s, box-shadow 0.2s;
}

/* 输入光标与文字跟放大镜对齐 */
.source-filter :deep(.ant-select-selection-search) {
  left: 30px;
  inset-inline-start: 30px;
}

.source-filter :deep(.ant-select-single .ant-select-selection-item),
.source-filter :deep(.ant-select-single .ant-select-selection-placeholder) {
  line-height: 34px;
}

.source-filter
  :deep(.ant-select:not(.ant-select-disabled):hover .ant-select-selector) {
  border-color: rgba(82, 117, 144, 0.9);
  background: rgba(128, 128, 128, 0.14);
}

.source-filter :deep(.ant-select-focused .ant-select-selector) {
  border-color: #527590;
  box-shadow: 0 0 0 2px rgba(82, 117, 144, 0.25);
}
</style>
