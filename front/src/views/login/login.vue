<template>
  <div class="login-page">
    <header class="login-header">
      <a class="login-brand" href="#/login"
        ><img :src="logoUrl" alt="" /><span>Yearning</span></a
      >
      <a-space>
        <span class="login-version">{{ Version }}</span>
        <a-button v-if="is_register" @click="is_open = true">{{
          $t('user.form.title')
        }}</a-button>
      </a-space>
    </header>
    <main class="login-main">
      <section class="login-intro">
        <span class="login-eyebrow">DATABASE WORKSPACE</span>
        <h1>{{ $t('common.login.headline') }}</h1>
        <p>{{ $t('common.login.description') }}</p>
        <div class="login-capabilities">
          <span>{{ $t('common.login.audit') }}</span>
          <span>{{ $t('common.login.query') }}</span>
          <span>{{ $t('common.login.access') }}</span>
        </div>
        <div class="login-databases"
          >MySQL <span>／</span> PostgreSQL <span>／</span> MongoDB
          <span>／</span> Redis</div
        >
      </section>
      <section class="login-panel" :aria-label="$t('common.signin')">
        <LoginForm />
      </section>
    </main>
    <footer class="login-footer">
      <div class="login-links">
        <a
          href="https://yearning.io"
          target="_blank"
          rel="noopener noreferrer"
          >{{ $t('common.about') }}</a
        >
        <a-tooltip :title="$t('common.qq')"
          ><button type="button">{{
            $t('common.community')
          }}</button></a-tooltip
        >
        <button type="button" @click="announce.open()">{{
          $t('common.statement')
        }}</button>
        <a
          href="https://www.gnu.org/licenses/agpl-3.0.en.html"
          target="_blank"
          rel="noopener noreferrer"
          >AGPL3.0</a
        >
      </div>
      <span>{{ Copyright }}</span>
    </footer>
    <Announce ref="announce" />
    <a-modal v-model:open="is_open" :title="$t('user.form.title')">
      <template #footer></template>
      <Register
        ref="register"
        :is-manager="false"
        @registered="is_open = false"
      />
    </a-modal>
  </div>
</template>

<script lang="ts" setup>
  import LoginForm from '@/views/login/login-form.vue';
  import { ref, onMounted } from 'vue';
  import { Copyright, Version } from '@/config/vars';
  import CommonMixin from '@/mixins/common';
  import Register from '@/components/user/registerForm.vue';
  import Announce from '../common/announce.vue';
  import { systemRegisterState } from '@/apis/loginApi';

  const logoUrl = `${import.meta.env.BASE_URL}icon.png`;

  const is_register = ref(false);

  const announce = ref();

  const register = ref();

  const { is_open } = CommonMixin();

  onMounted(async () => {
    const { data } = await systemRegisterState();
    is_register.value = data.payload.reg;
  });
</script>

<style scoped>
  .login-page {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    color: #e6edf7;
    background:
      radial-gradient(ellipse at 8% 25%, #1e3654 0%, transparent 54%), #101722;
    --focus-ring: #8eb6ff;
  }
  .login-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding: 28px clamp(24px, 5vw, 80px);
  }
  .login-brand {
    display: inline-flex;
    align-items: center;
    gap: 12px;
    color: #f4f7fc;
    font-size: 24px;
    font-weight: 650;
    letter-spacing: -0.5px;
  }
  .login-brand img {
    width: 32px;
    height: 32px;
  }
  .login-version {
    color: #a5b4c8;
    font-size: 13px;
  }
  .login-main {
    flex: 1;
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 420px);
    align-items: center;
    gap: clamp(32px, 7vw, 112px);
    width: min(1120px, 100%);
    margin: 0 auto;
    padding: 48px 32px 64px;
    box-sizing: border-box;
  }
  .login-eyebrow {
    color: #8eb6ff;
    font-size: 12px;
    font-weight: 600;
    letter-spacing: 2.5px;
  }
  .login-intro h1 {
    color: #f4f7fc;
    font-size: clamp(32px, 3.2vw, 48px);
    font-weight: 650;
    line-height: 1.3;
    letter-spacing: -1px;
    margin: 20px 0;
    text-wrap: balance;
  }
  .login-intro p {
    max-width: 480px;
    color: #a5b4c8;
    font-size: 16px;
    line-height: 1.9;
  }
  .login-capabilities {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    margin-top: 32px;
  }
  .login-capabilities span {
    padding: 6px 12px;
    border: 1px solid #3e526e;
    border-radius: 6px;
    color: #c7d5e8;
    font-size: 13px;
  }
  .login-databases {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 40px;
    font-size: 12px;
    color: #a5b4c8;
  }
  .login-databases span {
    color: #62758e;
  }
  .login-panel {
    min-width: 0;
    padding: 36px;
    border: 1px solid #3e526e;
    border-radius: 16px;
    background: #192332;
    box-shadow: 0 24px 64px #0003;
  }
  .login-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 16px;
    padding: 24px clamp(24px, 5vw, 80px);
    border-top: 1px solid #304056;
    color: #a5b4c8;
    font-size: 12px;
  }
  .login-links {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 20px;
  }
  .login-links a,
  .login-links button {
    color: #c7d5e8;
    font: inherit;
    background: transparent;
    border: 0;
    padding: 0;
    cursor: pointer;
  }
  .login-links a:hover,
  .login-links button:hover {
    color: #8eb6ff;
  }
  @media (max-width: 900px) {
    .login-main {
      grid-template-columns: minmax(0, 1fr) minmax(0, 380px);
      gap: 32px;
    }
    .login-intro h1 {
      font-size: 32px;
    }
    .login-panel {
      padding: 28px;
    }
  }
  @media (max-width: 700px) {
    .login-main {
      grid-template-columns: minmax(0, 420px);
      justify-content: center;
      padding: 24px 20px 40px;
      gap: 28px;
    }
    .login-intro {
      text-align: center;
    }
    .login-intro h1 {
      font-size: 28px;
      margin: 12px 0;
    }
    .login-intro p {
      font-size: 14px;
      margin-bottom: 0;
    }
    .login-capabilities,
    .login-databases {
      display: none;
    }
    .login-header {
      padding: 20px;
    }
    .login-brand {
      font-size: 20px;
    }
    .login-footer {
      justify-content: center;
      text-align: center;
      padding: 20px;
    }
    .login-panel {
      padding: 24px;
    }
  }
</style>
