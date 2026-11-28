<template>
  <a-page-header
    class="workspace-page-header"
    :title="props.title"
    :ghost="false"
    @back="() => $router.go(-1)"
  >
    <span class="page-description">{{ props.subTitle }}</span>
  </a-page-header>
</template>

<script lang="ts" setup>
  const props = defineProps<{
    title: string;
    subTitle: string;
  }>();
</script>

<style scoped>
  .workspace-page-header {
    margin-bottom: 20px;
    padding: 0 0 18px;
    border-bottom: 1px solid var(--frame-line);
    background: transparent;
  }
  .page-description {
    color: var(--text-muted);
    line-height: 1.6;
  }
</style>
