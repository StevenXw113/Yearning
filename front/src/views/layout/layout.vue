<template>
  <div v-watermark="{ text: store.state.user.account.user }">
    <a-layout style="min-height: 100vh">
      <!-- 窄屏（<992px）自动收起成 0 宽，靠顶栏的折叠图标展开，
           取代原来那个永远打不开的 drawer（is_open 全文件没人置 true） -->
      <a-layout-sider
        v-model:collapsed="collapsed"
        :trigger="null"
        collapsible
        breakpoint="lg"
        :collapsed-width="0"
        :width="200"
      >
        <div class="logo">
          <img :src="logoUrl" alt="Yearning" />
        </div>
        <Menu></Menu>
      </a-layout-sider>

      <a-layout>
        <a-row>
          <a-layout-header :style="{ zIndex: 1, width: '100%' }">
            <div class="header-bar">
              <div class="header-actions">
                <menu-unfold-outlined
                  v-if="collapsed"
                  class="trigger"
                  @click="() => (collapsed = !collapsed)"
                />
                <menu-fold-outlined
                  v-else
                  class="trigger"
                  @click="() => (collapsed = !collapsed)"
                />
                <a-button type="text" @click="toggle">
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
                  <a-button type="text" @click="changeTheme">
                    <!-- 图标库(icons-vue 7)没有 sun/moon，内联两条描边路径，颜色跟 currentColor -->
                    <template #icon>
                      <svg
                        v-if="isDark"
                        class="theme-icon"
                        viewBox="0 0 24 24"
                        aria-hidden="true"
                      >
                        <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" />
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
                <span class="user-entry user-name">{{
                  store.state.user.account.user
                }}</span>
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
          <a-layout-content
            :style="{ margin: '24px 16px 0', overflow: 'initial', minWidth: 0 }"
          >
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
  import { Copyright } from '@/config/vars';
  import Menu from '@/components/menu/menu.vue';
  import { useStore } from '@/store';
  import { useRouter } from 'vue-router';
  import {
    FullscreenOutlined,
    FullscreenExitOutlined,
    MenuUnfoldOutlined,
    MenuFoldOutlined,
  } from '@ant-design/icons-vue';
  import { useFullscreen } from '@vueuse/core';
  import { ref } from 'vue';

  // 只用系统图标（public/icon.png），不再用带文字的整张 logo，省掉那一段横向空白
  const logoUrl = `${import.meta.env.BASE_URL}icon.png`;

  const store = useStore();

  const router = useRouter();

  const collapsed = ref<boolean>(false);

  const isDark = ref(localStorage.getItem('theme') !== 'light');

  // 亮暗主题是构建期 less 变量，切换只能整页重载（与「个人中心」里的主题下拉一致）
  const changeTheme = () => {
    localStorage.setItem('theme', isDark.value ? 'light' : 'dark');
    location.reload();
  };

  const changeUser = (e: { key: string | number }) => {
    const key = e.key as string;
    if (key === '/exist') {
      sessionStorage.clear();
      store.state.user.account.token = '';
      router.push('/login');
    } else {
      router.push(key);
    }
  };

  const { isFullscreen, toggle } = useFullscreen();
</script>

<style scoped>
/* 只放图标：与菜单图标同一左内边距，上下留白收窄 */
.logo {
  display: flex;
  align-items: center;
  padding: 10px 12px 10px 32px; /* 32px = 菜单项胶囊内缩 8 + 内边距 24，图标与菜单图标同一条竖线 */
  margin-bottom: 6px;
  border-bottom: 1px solid var(--frame-line);
}
.logo img {
  display: block;
  width: 30px;
  height: 30px;
}
/* 收起时图标居中，与收起的菜单图标对齐 */
:deep(.ant-layout-sider-collapsed) .logo {
  justify-content: center;
  padding-left: 12px;
}


/* 去掉分隔线后靠底色区分：暗色主题下框架（侧栏+顶栏）比内容区(#1F262E)暗一档，
   原来的 #21262D 与内容区只差 1%，去掉线会糊成一片。
   取值走 CSS 变量，否则亮色主题下侧栏/顶栏也会是这块深色 */
:deep(.ant-layout-sider),
:deep(.ant-layout-header) {
  background: var(--frame-bg);
}

/* 展开态侧栏宽度按内容自适应（收起态仍用 antd 的 80px） */
:deep(.ant-layout-sider:not(.ant-layout-sider-collapsed)) {
  flex: 0 0 auto !important;
  width: auto !important;
  min-width: 0 !important;
  max-width: none !important;
}

/* 子菜单箭头紧跟标题文字：antd 默认把它绝对定位在列最右侧 */
:deep(.ant-menu-inline .ant-menu-submenu-title) {
  padding-right: 12px; /* 不再为浮动箭头预留 34px */
}
:deep(.ant-menu-inline .ant-menu-submenu-title .ant-menu-submenu-arrow) {
  position: static;
  transform: none;
  margin-left: 6px;
}

/* ── 侧边菜单：内缩圆角胶囊高亮，替掉 antd 默认的整条填充 ── */
:deep(.ant-menu-inline .ant-menu-item),
:deep(.ant-menu-inline .ant-menu-submenu-title) {
  width: auto;
  margin: 4px 8px;
  border-radius: 6px;
}
/* antd 选中项右侧的 3px 竖条，和胶囊样式冲突 */
:deep(.ant-menu-item-selected)::after {
  display: none;
}
:deep(.ant-menu-light .ant-menu-item:hover),
:deep(.ant-menu-light .ant-menu-submenu-title:hover) {
  background: var(--frame-hover);
}
/* 去掉菜单右侧 1px 分隔线；整列统一用侧栏底色（#21262D）与内容区（#1F262E）区分。
   .ant-menu 同时覆盖展开(inline)与收起(vertical)两种模式 */
:deep(.ant-menu) {
  border-right: none;
  border-inline-end: none;
  background: transparent;
}
/* 子菜单只要胶囊高亮，不要整块底色 */
:deep(.ant-menu-sub.ant-menu-inline) {
  background: transparent;
}
:deep(.ant-menu-item .anticon),
:deep(.ant-menu-submenu-title .anticon) {
  font-size: 15px;
}
/* 展开的分组：标题加一层淡淡底色，标出当前所在分组 */
:deep(.ant-menu-submenu-open > .ant-menu-submenu-title) {
  background: var(--frame-group);
}
/* 子项靠字色区分层级，选中/悬停时回到高对比 */
:deep(.ant-menu-sub .ant-menu-item) {
  color: var(--menu-sub-color);
}
:deep(.ant-menu-sub .ant-menu-item-selected),
:deep(.ant-menu-sub .ant-menu-item:hover) {
  color: var(--menu-sub-active);
}
:deep(.ant-menu-submenu-arrow) {
  opacity: 0.45;
}

/* 收起态宽度为 0 时 antd 会在左上角浮出一个箭头，顶栏已经有折叠图标了，去掉避免两个入口打架 */
:deep(.ant-layout-sider-zero-width-trigger) {
  display: none;
}

/* 顶栏：左侧操作区 + 右侧用户区，两端对齐，不再用栅格 offset 定位 */
.header-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 100%;
}
.header-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}
/* 折叠/全屏图标原本没有任何样式（12px、无手型） */
.trigger {
  font-size: 18px;
  cursor: pointer;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 4px;
}
.theme-icon {
  width: 1em;
  height: 1em;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.user-entry {
  cursor: pointer;
  padding: 0 8px;
  border-radius: 6px;
}
.user-entry:hover {
  background: var(--frame-hover-strong);
}
.user-name {
  font-weight: bold;
}
</style>
