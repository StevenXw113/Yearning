<template>
  <a-card>
    <UserTableSearch @search="search"></UserTableSearch>
    <!-- x 取各列宽度之和：容器不够宽时出现横向滚动，而不是把操作按钮挤出可视区 -->
    <c-table ref="tbl" :tbl-ref="tblRef" :scroll="{ x: 1170 }">
      <template #bodyCell="{ column, text, record }">
        <template v-if="column.dataIndex === 'action'">
          <!-- 所有操作按钮统一用 primary 描边：无 type 的 ghost 按钮在深色主题下几乎不可见 -->
          <a-space size="small" wrap>
            <a-button
              v-if="!record.editable"
              type="primary"
              size="small"
              ghost
              @click="() => (record.editable = true)"
              >{{ $t('common.edit') }}</a-button
            >
            <template v-else>
              <a-button
                type="primary"
                size="small"
                ghost
                @click="editUserInfo(record)"
                >{{ $t('common.save') }}
              </a-button>
              <a-button
                type="primary"
                size="small"
                ghost
                @click="() => (record.editable = false)"
                >{{ $t('common.cancel') }}</a-button
              >
            </template>
            <a-button
              type="primary"
              size="small"
              ghost
              @click="openPasswordModal(record.username)"
              >{{ $t('common.password') }}</a-button
            >
            <a-button
              type="primary"
              size="small"
              ghost
              @click="openRuleModal(record.username)"
            >
              {{ $t('common.policy') }}</a-button
            >
            <a-popconfirm
              :title="$t('user.mfa.reset.tips')"
              @confirm="resetMfa(record.username)"
            >
              <a-button type="primary" size="small" ghost>
                {{ $t('user.mfa.reset') }}
              </a-button>
            </a-popconfirm>
            <a-popconfirm
              :title="$t('user.form.delete.tips')"
              @confirm="deleteUser(record.username)"
            >
              <a-button
                v-if="record.username !== 'admin'"
                type="primary"
                danger
                ghost
                size="small"
                >{{ $t('common.delete') }}</a-button
              >
            </a-popconfirm>
          </a-space>
        </template>
        <template
          v-if="
            ['real_name', 'department', 'email', 'is_recorder'].includes(
              column.dataIndex,
            )
          "
        >
          <template v-if="!record.editable">
            <span v-if="column.dataIndex === 'is_recorder'">{{
              text === 2 ? $t('common.no') : $t('common.yes')
            }}</span>
            <span v-else>{{ text }}</span>
          </template>
          <template v-else>
            <a-select
              v-if="column.dataIndex === 'is_recorder'"
              v-model:value="record[column.dataIndex]"
              :options="[
                {
                  label: $t('common.no'),
                  value: 2,
                  key: 2,
                  title: $t('common.no'),
                },
                {
                  label: $t('common.yes'),
                  value: 1,
                  key: 1,
                  title: $t('common.yes'),
                },
              ]"
            >
            </a-select>
            <a-input v-else v-model:value="record[column.dataIndex]"></a-input>
          </template>
        </template>
      </template>
    </c-table>
  </a-card>

  <ChangePassword
    v-model:open="isOpen"
    is-manager
    @post="handlePassword"
  ></ChangePassword>
  <UserRules ref="r" is-manager></UserRules>
</template>

<script lang="ts" setup>
  import UserTableSearch from './userTableSearch.vue';
  import UserRules from '@/components/user/userRules.vue';
  import { ref, reactive } from 'vue';
  import {
    UserExpr,
    UserTableData,
    RegisterForm,
    Password,
    updatePassword,
    deleteUserAccount,
    getUserList,
    updateUserInfo,
    resetUserMfa,
  } from '@/apis/user';
  import ChangePassword from '@/components/user/changePassword.vue';
  import { App } from 'ant-design-vue';
  const { message } = App.useApp();
  import { useI18n } from 'vue-i18n';
  import { tableRef } from '@/components/table';
  import { CommonPage } from '@/types';

  const { t } = useI18n();

  const tblRef = reactive<tableRef>({
    col: [
      {
        title: t('user.form.user'),
        dataIndex: 'username',
      },
      {
        title: t('user.form.real_name'),
        dataIndex: 'real_name',
      },
      {
        title: t('user.form.dept'),
        dataIndex: 'department',
      },
      {
        title: t('user.form.email'),
        dataIndex: 'email',
      },
      {
        title: t('user.form.recorder'),
        dataIndex: 'is_recorder',
      },
      {
        title: t('common.action'),
        dataIndex: 'action',
        // 5 个操作按钮（编辑/密码/权限/重置MFA/删除）需要足够宽度，
        // 否则 auto 布局下被其它列挤压、按钮跑到可视区外
        width: 420,
      },
    ],
    data: [] as UserTableData[],
    pageCount: 0,
    expr: {
      dept: '',
      username: '',
      email: '',
      real_name: '',
    } as UserExpr,
    fn: async (expr: CommonPage<UserExpr>) => {
      const { data } = await getUserList(expr);
      tblRef.data = data.payload.data;
      tblRef.pageCount = data.payload.page;
    },
  });

  const isOpen = ref(false);

  const r = ref();

  const account = ref('');

  const tbl = ref();

  const openPasswordModal = (user: string) => {
    isOpen.value = !isOpen.value;
    account.value = user;
  };

  const openRuleModal = (user: string) => {
    r.value.lazy(user);
    r.value.turnState();
  };

  const is_edit = ref(false);

  const resetMfa = async (user: string) => {
    const { data } = await resetUserMfa(user);
    if (data.code === 1200) {
      message.success(data.text);
    } else {
      message.error(data.text);
    }
  };

  const deleteUser = async (user: string) => {
    await deleteUserAccount(user);
    tbl.value.manual();
  };

  const search = (vl: UserExpr) => {
    tblRef.expr = vl;
    tbl.value.manual(true);
  };

  const editUserInfo = async (user: RegisterForm) => {
    await updateUserInfo(user, true);
    tbl.value.manual();
    is_edit.value = false;
  };

  const handlePassword = async (formItem: Password) => {
    await updatePassword({
      password: formItem,
      username: account.value,
      isManager: true,
    });
    isOpen.value = false;
  };

  // 供父组件中继调用：新增用户(注册表单提交)后刷新列表
  const refresh = () => {
    tbl.value?.manual();
  };

  defineExpose({
    refresh,
  });
</script>

<style scoped></style>
