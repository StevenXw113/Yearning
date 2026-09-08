<template>
  <a-drawer
    v-bind="$attrs"
    :visible="isDrawerVisible"
    :width="500"
    :header-style="{ display: 'none' }"
    :body-style="{ padding: 0 }"
    destroy-on-close
  >
    <orderProfile @close="() => emit('closeDrawer')" />
  </a-drawer>
</template>

<script setup lang="ts">
  import { ref } from 'vue';
  import orderProfile from './orderProfile.vue';
  const emit = defineEmits(['closeDrawer']);
  const isDrawerVisible = ref(true);
</script>

<style scoped></style>
