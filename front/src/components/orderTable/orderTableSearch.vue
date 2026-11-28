<template>
  <a-form
    :name="formName"
    :model="expr"
    layout="vertical"
    class="order-search"
    @finish="search"
  >
    <div class="order-search-main">
      <div class="order-search-fields">
        <a-form-item :label="$t('common.table.post.time')" name="picker">
          <a-range-picker
            v-model:value="expr.picker"
            :presets="presets"
            format="YYYY-MM-DD HH:mm"
            show-time
          />
        </a-form-item>
        <a-form-item :label="$t('common.table.type')" name="type">
          <a-select v-model:value="expr.type">
            <a-select-option :value="2">{{
              $t('order.state.all')
            }}</a-select-option>
            <a-select-option :value="0">DDL</a-select-option>
            <a-select-option :value="1">DML</a-select-option>
          </a-select>
        </a-form-item>
      </div>
      <div class="order-search-actions">
        <a-button type="primary" html-type="submit"
          ><SearchOutlined aria-hidden="true" />{{
            $t('common.search')
          }}</a-button
        >
        <a-button @click="reset"
          ><ReloadOutlined aria-hidden="true" />{{
            $t('order.filter.reset')
          }}</a-button
        >
        <a-button
          type="text"
          :aria-expanded="advanced"
          :aria-controls="advancedId"
          @click="advanced = !advanced"
        >
          {{
            advanced ? $t('order.filter.collapse') : $t('order.filter.advanced')
          }}
          <UpOutlined v-if="advanced" aria-hidden="true" /><DownOutlined
            v-else
            aria-hidden="true"
          />
        </a-button>
      </div>
    </div>
    <div v-if="advanced" :id="advancedId" class="order-search-advanced">
      <a-form-item :label="$t('common.table.post.user')" name="username"
        ><a-input v-model:value="expr.username" allow-clear
      /></a-form-item>
      <a-form-item :label="$t('common.table.remark')" name="text"
        ><a-input v-model:value="expr.text" allow-clear
      /></a-form-item>
      <a-form-item :label="$t('common.table.state')" name="status"
        ><a-select v-model:value="expr.status">
          <a-select-option :value="8"
            >{{ $t('order.state.all') }}
          </a-select-option>
          <a-select-option :value="OrderState.WAIT"
            >{{ $t('order.state.wait') }}
          </a-select-option>
          <a-select-option :value="OrderState.PROCESS">{{
            $t('order.state.process')
          }}</a-select-option>
          <a-select-option :value="OrderState.AUDIT"
            >{{ $t('order.state.audit') }}
          </a-select-option>
          <a-select-option :value="OrderState.SUCCESS">{{
            $t('order.state.success')
          }}</a-select-option>
          <a-select-option :value="OrderState.REJECT">{{
            $t('order.state.reject')
          }}</a-select-option>
          <a-select-option :value="OrderState.ERROR"
            >{{ $t('order.state.error') }}
          </a-select-option>
          <a-select-option :value="OrderState.Undo"
            >{{ $t('order.undo') }}
          </a-select-option>
          <a-select-option :value="OrderState.Terminate"
            >{{ $t('order.terminate') }}
          </a-select-option>
        </a-select></a-form-item
      >
      <a-form-item :label="$t('common.table.work_id')" name="work_id"
        ><a-input v-model:value="expr.work_id" allow-clear
      /></a-form-item>
    </div>
  </a-form>
</template>

<script lang="ts" setup>
  import { ref, useId } from 'vue';
  import {
    SearchOutlined,
    ReloadOutlined,
    UpOutlined,
    DownOutlined,
  } from '@ant-design/icons-vue';
  import { useI18n } from 'vue-i18n';
  import dayjs from 'dayjs';
  import { OrderExpr } from '@/apis/orderPostApis';
  import { OrderState } from '@/types';

  const { t } = useI18n();
  const formName = 'order-search-' + useId();
  const advancedId = formName + '-advanced';
  const advanced = ref(false);
  const presets = [
    {
      label: t('order.filter.this.week'),
      value: [dayjs().startOf('week'), dayjs().endOf('week')],
    },
    {
      label: t('order.filter.this.month'),
      value: [dayjs().startOf('month'), dayjs().endOf('month')],
    },
  ];
  const initialExpr = (): OrderExpr => ({
    status: 8,
    type: 2,
    text: '',
    username: '',
    work_id: '',
  });
  const expr = ref<OrderExpr>(initialExpr());
  const emit = defineEmits<{ search: [expr: OrderExpr] }>();

  const search = () => {
    // 保留日期控件的 Dayjs 值，仅在提交时格式化查询参数。
    emit('search', {
      ...expr.value,
      picker: expr.value.picker?.map((value) =>
        dayjs(value).format('YYYY-MM-DD HH:mm'),
      ),
    });
  };
  const reset = () => {
    expr.value = initialExpr();
    search();
  };
</script>

<style lang="less" scoped>
  .order-search {
    padding-bottom: 20px;
    margin-bottom: 20px;
    border-bottom: 1px solid var(--border-split);
  }
  .order-search-main {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: 16px 24px;
  }
  .order-search-fields {
    display: grid;
    grid-template-columns: minmax(280px, 360px) minmax(140px, 180px);
    gap: 16px;
    flex: 1 1 480px;
  }
  .order-search :deep(.ant-form-item) {
    margin-bottom: 0;
    min-width: 0;
  }
  .order-search :deep(.ant-form-item-label) {
    padding-bottom: 6px;
  }
  .order-search :deep(.ant-picker) {
    width: 100%;
  }
  .order-search-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .order-search-advanced {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 16px;
    padding-top: 20px;
  }
  @media (max-width: 767px) {
    .order-search-fields,
    .order-search-advanced {
      grid-template-columns: minmax(0, 1fr);
      flex-basis: 100%;
    }
    .order-search-actions {
      width: 100%;
    }
    .order-search-actions :deep(.ant-btn) {
      min-height: 44px;
    }
    .order-search-actions :deep(.ant-btn-primary) {
      flex: 1;
    }
  }
</style>
