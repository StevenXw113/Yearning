<template>
  <div v-watermark="{ text: store.state.user.account.user }">
    <a-layout style="min-height: 100vh">
      <a-layout-sider
        v-model:collapsed="collapsed"
        :trigger="null"
        collapsible
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
              <a-dropdown>
                <a-space class="user-entry">
                  <a-avatar :src="profile" />
                  <span class="user-name">{{
                    store.state.user.account.user
                  }}</span>
                </a-space>
                <template #overlay>
                  <a-menu @click="() => router.push({ path: '/home/profile' })">
                    <a-menu-item>
                      <a href="javascript:;">{{
                        $t('common.profile.title')
                      }}</a>
                    </a-menu-item>
                  </a-menu>
                </template>
              </a-dropdown>
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
    <a-drawer
      placement="right"
      :closable="false"
      :visible="is_open"
      @close="close"
    >
      <Menu @close="() => (is_open = false)"></Menu>
    </a-drawer>
  </div>
</template>

<script setup lang="ts">
  import { Copyright } from '@/config/vars';
  import CommonMixin from '@/mixins/common';
  import Menu from '@/components/menu/menu.vue';
  import { useStore } from '@/store';
  import profile from '@/assets/comment/3.svg';
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

  const { is_open, close } = CommonMixin();

  const store = useStore();

  const router = useRouter();

  const collapsed = ref<boolean>(false);

  const { isFullscreen, toggle } = useFullscreen();
</script>

<style scoped>
/* 只放图标：与菜单图标同一左内边距，上下留白收窄 */
.logo {
  display: flex;
  align-items: center;
  padding: 10px 12px 10px 32px; /* 32px = 菜单项胶囊内缩 8 + 内边距 24，图标与菜单图标同一条竖线 */
  margin-bottom: 6px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
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


/* 去掉分隔线后靠底色区分：框架（侧栏+顶栏）比内容区(#1F262E)暗一档，
   原来的 #21262D 与内容区只差 1%，去掉线会糊成一片 */
:deep(.ant-layout-sider),
:deep(.ant-layout-header) {
  background: #1a1f26;
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
  background: rgba(255, 255, 255, 0.06);
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
  background: rgba(255, 255, 255, 0.04);
}
/* 子项靠字色区分层级，选中/悬停时回到高对比 */
:deep(.ant-menu-sub .ant-menu-item) {
  color: rgba(255, 255, 255, 0.7);
}
:deep(.ant-menu-sub .ant-menu-item-selected),
:deep(.ant-menu-sub .ant-menu-item:hover) {
  color: rgba(255, 255, 255, 0.95);
}
:deep(.ant-menu-submenu-arrow) {
  opacity: 0.45;
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

.user-entry {
  cursor: pointer;
  padding: 0 8px;
  border-radius: 6px;
}
.user-entry:hover {
  background: rgba(255, 255, 255, 0.08);
}
.user-name {
  font-weight: bold;
}
</style>
