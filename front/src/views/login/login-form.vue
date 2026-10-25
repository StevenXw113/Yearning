<template>
  <img src="../../assets/login/logo.png" width="350" />
  <br />
  <!-- 第二步：MFA 动态码（第一步密码通过且账号启用了 MFA 时显示） -->
  <a-form v-if="mfaRequired" :model="mfaForm">
    <a-form-item>
      <a-typography-text style="color: rgba(255, 255, 255, 0.75)">
        {{ $t('user.mfa.login.tips') }}（{{ mfaForm.user }}）
      </a-typography-text>
    </a-form-item>
    <a-form-item>
      <a-input
        v-model:value="mfaForm.code"
        :placeholder="$t('user.mfa.code')"
        style="border-radius: 10px; letter-spacing: 4px"
        :maxlength="6"
        @press-enter="() => mfaConfirm()"
      />
    </a-form-item>
    <a-button type="dashed" block ghost @click="mfaConfirm">{{
      $t('common.signin')
    }}</a-button>
    <a-button type="text" block style="margin-top: 10px" @click="backToLogin">
      {{ $t('user.mfa.back') }}
    </a-button>
  </a-form>
  <!-- 第一步：账号密码 -->
  <a-form v-else :model="loginForm">
    <a-form-item>
      <a-input
        v-model:value="loginForm.username"
        :placeholder="$t('user.username')"
        style="border-radius: 10px"
      />
    </a-form-item>
    <a-form-item>
      <a-input
        v-model:value="loginForm.password"
        :placeholder="$t('user.password')"
        style="border-radius: 10px"
        type="password"
        @press-enter="() => userSignIn()"
      />
    </a-form-item>
    <a-button type="dashed" block ghost @click="userSignIn">{{
      $t('common.signin')
    }}</a-button>
    <a-button
      v-if="oidcEnabled"
      type="dashed"
      block
      ghost
      style="margin-top: 10px"
      @click="oidcSignIn"
      >OIDC {{ $t('common.signin') }}
    </a-button>
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
    const { data } = await signIn(loginForm);
    if (data.code === 1301) {
      return;
    }
    // 密码正确但启用了 MFA：不发正式 token，进入第二步
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
  }, 200);

  const mfaConfirm = useDebounceFn(async () => {
    if (!mfaForm.code.trim()) return;
    const { data } = await mfaSignIn(mfaForm.ticket, mfaForm.code.trim());
    if (data.code !== 1200) {
      // 动态码错误/票据过期：保留输入视图让用户重试（错误次数由后端限流锁定）
      message.error(data.text);
      return;
    }
    userStore.USER_STORE(data.payload);
    menuStore.CHANGE_SELECTED(['/home']);
    router.replace('/home');
  }, 200);

  const backToLogin = () => {
    mfaRequired.value = false;
    mfaForm.ticket = '';
    mfaForm.code = '';
  };
</script>

<style lang="less" scoped>
  .ant-input {
    box-sizing: border-box;
    margin: 0;
    padding: 0;
    font-variant: tabular-nums;
    list-style: none;
    font-feature-settings: 'tnum';
    position: relative;
    display: inline-block;
    width: 100%;
    min-width: 0;
    padding: 4px 11px;
    color: rgba(255, 255, 255, 0.85);
    font-size: 14px;
    line-height: 1.5715;
    background-color: rgba(255, 255, 255, 0);
    background-image: none;
    border: 1px solid rgba(255, 255, 255, 0.09);
    border-radius: 2px;
    transition: all 0.3s;
  }

  .ant-btn.ant-btn-background-ghost {
    color: rgba(255, 255, 255, 0.85);
    border-color: rgba(255, 255, 255, 0.25);
  }
</style>
