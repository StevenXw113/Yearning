<template>
  <div class="login-form-heading">
    <h2>{{ mfaRequired ? $t('user.mfa.code') : $t('common.signin') }}</h2>
    <p>{{
      mfaRequired ? $t('user.mfa.login.tips') : $t('common.login.welcome')
    }}</p>
  </div>
  <a-form
    v-if="mfaRequired"
    :model="mfaForm"
    layout="vertical"
    @finish="mfaConfirm"
  >
    <p class="mfa-account">{{ mfaForm.user }}</p>
    <a-form-item
      :label="$t('user.mfa.code')"
      name="code"
      :rules="[{ required: true, message: $t('user.mfa.code') }]"
    >
      <a-input
        v-model:value="mfaForm.code"
        :placeholder="$t('user.mfa.code')"
        class="mfa-code"
        :maxlength="6"
        autocomplete="one-time-code"
        inputmode="numeric"
      />
    </a-form-item>
    <a-button type="primary" block html-type="submit" :loading="submitting">{{
      $t('common.signin')
    }}</a-button>
    <a-button
      type="text"
      block
      class="secondary-login"
      :disabled="submitting"
      @click="backToLogin"
      >{{ $t('user.mfa.back') }}</a-button
    >
  </a-form>
  <a-form v-else :model="loginForm" layout="vertical" @finish="userSignIn">
    <a-form-item
      :label="$t('user.username')"
      name="username"
      :rules="[
        { required: true, whitespace: true, message: $t('user.username') },
      ]"
    >
      <a-input
        v-model:value="loginForm.username"
        :placeholder="$t('user.username')"
        autocomplete="username"
      />
    </a-form-item>
    <a-form-item
      :label="$t('user.password')"
      name="password"
      :rules="[{ required: true, message: $t('user.password') }]"
    >
      <a-input-password
        v-model:value="loginForm.password"
        :placeholder="$t('user.password')"
        autocomplete="current-password"
      />
    </a-form-item>
    <a-button type="primary" block html-type="submit" :loading="submitting">{{
      $t('common.signin')
    }}</a-button>
    <a-button
      v-if="oidcEnabled"
      block
      class="secondary-login"
      :disabled="submitting"
      @click="oidcSignIn"
      >OIDC {{ $t('common.signin') }}</a-button
    >
  </a-form>
</template>

<script setup lang="ts">
  import { UnwrapRef, reactive, ref, onMounted, computed } from 'vue';
  import { signIn, mfaSignIn, LoginFrom, getOIDCState } from '@/apis/loginApi';
  import router from '@/router';
  import { useUserStore, useMenuStore } from '@/store';
  import { useRoute } from 'vue-router';
  import { useDebounceFn } from '@vueuse/core';
  import { App } from 'ant-design-vue';
  const { message } = App.useApp();

  const loginForm: UnwrapRef<LoginFrom> = reactive({
    username: '',
    password: '',
    is_oidc: false,
  });

  // MFA 第二步状态：第一步成功但 payload.mfa_required 时切换视图
  const mfaRequired = ref(false);
  const submitting = ref(false);
  const mfaForm = reactive({ ticket: '', code: '', user: '' });

  const userStore = useUserStore();
  const menuStore = useMenuStore();
  const route = useRoute();

  const oidcEnabled = ref(false);
  const oidcSignInUrl = ref('');

  const query = computed(() => route.query).value;

  const fetchOIDC = async () => {
    const { data } = await getOIDCState();
    if (data.code == 1200 && data.payload.enabled && data.payload.authUrl) {
      oidcEnabled.value = true;
      oidcSignInUrl.value = data.payload.authUrl;
    }
  };

  onMounted(() => {
    fetchOIDC();
    if (query.oidcLogin) {
      // OIDC 回调把登录态放在查询参数里（token/user/real_name/is_record）。
      // 显式构造 LoginRespPayload：route.query 的值是 string|string[]，
      // 直接透传曾让 is_record 以字符串入 store，导致 is_record === 1 判断失效。
      userStore.USER_STORE({
        token: String(query.token ?? ''),
        user: String(query.user ?? ''),
        real_name: String(query.real_name ?? ''),
        is_record: Number(query.is_record ?? 2),
        rule: String(query.rule ?? ''),
      });
      menuStore.CHANGE_SELECTED(['/home']);
      router.replace('/home');
    }
  });

  const oidcSignIn = () => {
    window.location.href = oidcSignInUrl.value;
  };

  const userSignIn = useDebounceFn(async () => {
    if (submitting.value) return;
    submitting.value = true;
    try {
      const { data } = await signIn(loginForm);
      if (data.code !== 1200) return;
      if (data.payload?.mfa_required) {
        mfaForm.ticket = data.payload.ticket;
        mfaForm.user = data.payload.user || loginForm.username;
        mfaForm.code = '';
        mfaRequired.value = true;
        return;
      }
      userStore.USER_STORE(data.payload);
      menuStore.CHANGE_SELECTED(['/home']);
      router.replace('/home');
    } finally {
      submitting.value = false;
    }
  }, 200);

  const mfaConfirm = useDebounceFn(async () => {
    if (submitting.value || !mfaForm.code.trim()) return;
    submitting.value = true;
    try {
      const { data } = await mfaSignIn(mfaForm.ticket, mfaForm.code.trim());
      if (data.code !== 1200) {
        message.error(data.text);
        return;
      }
      userStore.USER_STORE(data.payload);
      menuStore.CHANGE_SELECTED(['/home']);
      router.replace('/home');
    } finally {
      submitting.value = false;
    }
  }, 200);

  const backToLogin = () => {
    mfaRequired.value = false;
    mfaForm.ticket = '';
    mfaForm.code = '';
  };
</script>

<style lang="less" scoped>
  .login-form-heading {
    margin-bottom: 28px;
  }
  .login-form-heading h2 {
    margin: 0 0 8px;
    color: #f4f7fc;
    font-size: 24px;
    font-weight: 600;
  }
  .login-form-heading p,
  .mfa-account {
    color: #a5b4c8;
    margin: 0;
    font-size: 14px;
  }
  :deep(.ant-form-item-label > label) {
    color: #c7d5e8;
  }
  :deep(.ant-input),
  :deep(.ant-input-affix-wrapper) {
    min-height: 44px;
    color: #e6edf7;
    background: #101722;
    border-color: #62758e;
    border-radius: 8px;
  }
  :deep(.ant-input-affix-wrapper .ant-input) {
    min-height: 0;
  }
  :deep(.ant-input::placeholder) {
    color: #a5b4c8;
  }
  :deep(.ant-input-password-icon) {
    color: #a5b4c8;
  }
  :deep(.ant-input:focus),
  :deep(.ant-input-affix-wrapper-focused) {
    border-color: #8eb6ff;
    box-shadow: 0 0 0 3px #6b9eff26;
  }
  :deep(.ant-btn) {
    min-height: 44px;
    font-weight: 500;
  }
  :deep(.ant-btn-primary) {
    background: #245dd8;
    border-color: #245dd8;
    color: #fff;
  }
  :deep(.ant-btn-primary:not(:disabled):hover) {
    background: #1e50bd;
    border-color: #1e50bd;
  }
  .secondary-login {
    margin-top: 12px;
    color: #c7d5e8;
    background: #202d3e;
    border-color: #62758e;
  }
  .mfa-account {
    margin-bottom: 16px;
  }
  .mfa-code {
    letter-spacing: 4px;
    font-variant-numeric: tabular-nums;
  }
</style>
