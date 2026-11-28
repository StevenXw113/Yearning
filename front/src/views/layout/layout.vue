<template>
  <div v-watermark="{ text: userStore.account.user }">
    <a-layout class="workspace-layout" style="min-height: 100vh">
      <!-- 窄屏（<992px）自动收起成 0 宽，靠顶栏的折叠图标展开，
           取代原来那个永远打不开的 drawer（is_open 全文件没人置 true） -->
      <a-layout-sider
        v-model:collapsed="collapsed"
        :trigger="null"
        collapsible
        breakpoint="lg"
        :collapsed-width="0"
        :width="224"
      >
        <div class="logo">
          <img :src="logoUrl" alt="" />
          <span>Yearning</span>
        </div>
        <nav
          id="workspace-navigation"
          :aria-label="$t('common.navigation.label')"
          ><Menu @close="closeMobileMenu"
        /></nav>
      </a-layout-sider>
      <button
        v-if="!collapsed"
        class="navigation-backdrop"
        type="button"
        :aria-label="$t('common.navigation.toggle')"
        @click="collapsed = true"
      />

      <a-layout>
        <a-row>
          <a-layout-header :style="{ zIndex: 1, width: '100%' }">
            <div class="header-bar">
              <div class="header-actions">
                <a-button
                  type="text"
                  :aria-label="$t('common.navigation.toggle')"
                  :aria-expanded="!collapsed"
                  aria-controls="workspace-navigation"
                  @click="collapsed = !collapsed"
                >
                  <template #icon>
                    <MenuUnfoldOutlined v-if="collapsed" />
                    <MenuFoldOutlined v-else />
                  </template>
                </a-button>
                <a-button
                  type="text"
                  :aria-label="$t('common.fullscreen.toggle')"
                  :aria-pressed="isFullscreen"
                  @click="toggle"
                >
                  <template v-if="isFullscreen" #icon>
                    <FullscreenExitOutlined />
                  </template>
                  <template v-else #icon>
                    <FullscreenOutlined />
                  </template>
                </a-button>
              </div>
              <div class="header-right">
                <a-tooltip
                  :title="
                    isDark ? $t('common.theme.light') : $t('common.theme.dark')
                  "
                >
                  <a-button
                    type="text"
                    :aria-label="
                      isDark
                        ? $t('common.theme.light')
                        : $t('common.theme.dark')
                    "
                    @click="changeTheme"
                  >
                    <!-- 图标库(icons-vue 7)没有 sun/moon，内联两条描边路径，颜色跟 currentColor -->
                    <template #icon>
                      <svg
                        v-if="isDark"
                        class="theme-icon"
                        viewBox="0 0 24 24"
                        aria-hidden="true"
                      >
                        <path
                          d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"
                        />
                      </svg>
                      <svg
                        v-else
                        class="theme-icon"
                        viewBox="0 0 24 24"
                        aria-hidden="true"
                      >
                        <circle cx="12" cy="12" r="4.2" />
                        <path
                          d="M12 2v2.4M12 19.6V22M2 12h2.4M19.6 12H22M4.9 4.9l1.7 1.7M17.4 17.4l1.7 1.7M4.9 19.1l1.7-1.7M17.4 6.6l1.7-1.7"
                        />
                      </svg>
                    </template>
                  </a-button>
                </a-tooltip>
                <a-dropdown>
                  <button type="button" class="user-entry user-name">{{
                    userStore.account.user
                  }}</button>
                  <template #overlay>
                    <a-menu @click="changeUser">
                      <a-menu-item key="/home/profile">
                        <a href="javascript:;">{{
                          $t('common.profile.title')
                        }}</a>
                      </a-menu-item>
                      <a-menu-item key="/exist">
                        <a href="javascript:;">{{ $t('menu.loginout') }}</a>
                      </a-menu-item>
                    </a-menu>
                  </template>
                </a-dropdown>
              </div>
            </div>
          </a-layout-header>
        </a-row>
        <a-row>
          <!-- minWidth:0 让 flex 项可以收缩，否则宽表格会把整页顶出横向滚动条 -->
          <a-layout-content class="workspace-content">
            <router-view v-slot="{ Component }">
              <component :is="Component" />
            </router-view>
          </a-layout-content>
        </a-row>

        <a-layout-footer :style="{ textAlign: 'center', width: '100%' }">
          <a-space>
            <span>{{ Copyright }}</span>
            <a href="https://next.yearning.io" target="_blank">{{
              $t('common.help')
            }}</a>
          </a-space>
        </a-layout-footer>
      </a-layout>
    </a-layout>
  </div>
</template>

<script setup lang="ts">
  import { toggleTheme, isDark as themeIsDark } from '@/style/theme';
  import { Copyright } from '@/config/vars';
  import Menu from '@/components/menu/menu.vue';
  import { useUserStore } from '@/store';
  import { useRouter } from 'vue-router';
  import {
    FullscreenOutlined,
    FullscreenExitOutlined,
    MenuUnfoldOutlined,
    MenuFoldOutlined,
  } from '@ant-design/icons-vue';
  import { useFullscreen } from '@vueuse/core';
  import { computed, ref } from 'vue';

  // 只用系统图标（public/icon.png），不再用带文字的整张 logo，省掉那一段横向空白
  const logoUrl = `${import.meta.env.BASE_URL}icon.png`;

  const userStore = useUserStore();

  const router = useRouter();

  const collapsed = ref<boolean>(false);

  // antd 4 主题运行时切换（CSS-in-JS token + data-theme CSS 变量），不再整页重载
  const isDark = computed(() => themeIsDark.value === 'dark');

  const closeMobileMenu = () => {
    if (window.matchMedia('(max-width: 991px)').matches) collapsed.value = true;
  };

  const changeTheme = () => {
    toggleTheme();
  };

  const changeUser = (e: { key: string | number }) => {
    const key = e.key as string;
    if (key === '/exist') {
      sessionStorage.clear();
      userStore.account.token = '';
      router.push('/login');
    } else {
      router.push(key);
    }
  };

  const { isFullscreen, toggle } = useFullscreen();
</script>

<style scoped>
  .workspace-layout {
    min-width: 0;
  }
  .navigation-backdrop {
    display: none;
  }
  .logo {
    display: flex;
    align-items: center;
    gap: 12px;
    min-height: 72px;
    padding: 16px 24px;
    border-bottom: 1px solid var(--frame-line);
  }
  .logo img {
    width: 30px;
    height: 30px;
  }
  .logo span {
    font-size: 20px;
    font-weight: 650;
    letter-spacing: -0.5px;
    color: var(--total-color);
  }
  :deep(.ant-layout-sider),
  :deep(.ant-layout-header) {
    background: var(--frame-bg);
  }
  :deep(.ant-layout-sider) {
    border-right: 1px solid var(--frame-line);
  }
  :deep(.ant-layout-sider-children) {
    overflow-y: auto;
  }
  :deep(.ant-layout-header) {
    height: 64px;
    padding: 0 24px;
    border-bottom: 1px solid var(--frame-line);
  }
  :deep(.ant-menu) {
    background: transparent;
    border-inline-end: 0;
  }
  :deep(.ant-menu-sub.ant-menu-inline) {
    background: transparent;
  }
  :deep(.ant-menu-inline .ant-menu-item),
  :deep(.ant-menu-inline .ant-menu-submenu-title) {
    width: auto;
    margin: 4px 12px;
    border-radius: 8px;
  }
  :deep(.ant-menu-item-selected)::after {
    display: none;
  }
  :deep(.ant-menu-item .anticon),
  :deep(.ant-menu-submenu-title .anticon) {
    font-size: 16px;
  }
  :deep(.ant-menu-sub .ant-menu-item) {
    color: var(--menu-sub-color);
  }
  :deep(.ant-menu-sub .ant-menu-item-selected) {
    color: var(--menu-sub-active);
  }
  :deep(.ant-layout-sider-zero-width-trigger) {
    display: none;
  }
  .workspace-content {
    width: 100%;
    margin: 24px;
    min-width: 0;
  }
  .header-bar,
  .header-actions,
  .header-right {
    display: flex;
    align-items: center;
  }
  .header-bar {
    justify-content: space-between;
    height: 100%;
    gap: 12px;
  }
  .header-actions,
  .header-right {
    gap: 8px;
  }
  .header-bar :deep(.ant-btn) {
    min-width: 40px;
    height: 40px;
  }
  .theme-icon {
    width: 18px;
    height: 18px;
    fill: none;
    stroke: currentColor;
    stroke-width: 1.8;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
  .user-entry {
    border: 0;
    border-radius: 6px;
    padding: 8px 12px;
    background: transparent;
    color: inherit;
    font: inherit;
    cursor: pointer;
  }
  .user-entry:hover {
    background: var(--surface-muted);
  }
  .user-name {
    max-width: 180px;
    overflow: hidden;
    text-overflow: ellipsis;
    font-weight: 600;
  }
  @media (max-width: 991px) {
    :deep(.ant-layout-sider) {
      position: fixed;
      top: 64px;
      bottom: 0;
      left: 0;
      z-index: 20;
    }
    .navigation-backdrop {
      display: block;
      position: fixed;
      inset: 64px 0 0;
      z-index: 10;
      border: 0;
      background: #0006;
      cursor: pointer;
    }
    .workspace-content {
      margin: 20px 16px;
    }
    :deep(.ant-layout-header) {
      padding: 0 16px;
    }
  }
  @media (max-width: 575px) {
    .workspace-content {
      margin: 16px 12px;
    }
    .header-actions,
    .header-right {
      gap: 4px;
    }
    :deep(.ant-layout-header) {
      padding: 0 12px;
    }
  }
</style>
