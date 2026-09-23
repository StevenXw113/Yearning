<template>
  <div class="assistant">
    <a-alert
      v-if="error"
      type="error"
      :message="error"
      closable
      style="margin-bottom: 12px"
      @close="error = ''"
    />
    <div ref="listRef" class="assistant-list">
      <div v-if="!messages.length" class="assistant-empty">
        {{ $t('order.assistant.empty') }}
      </div>
      <div
        v-for="(m, i) in messages"
        :key="i"
        class="assistant-row"
        :class="m.role"
      >
        <!-- 流式过程中按纯文本展示；结束后才渲染 markdown（清洗后再挂载） -->
        <div v-if="m.html" class="assistant-bubble md" v-html="m.html"></div>
        <div v-else class="assistant-bubble">
          {{ m.content }}<span v-if="m.streaming" class="assistant-caret"></span>
        </div>
      </div>
    </div>
    <div class="assistant-input">
      <a-textarea
        v-model:value="input"
        :rows="3"
        :placeholder="$t('order.assistant.placeholder')"
        @press-enter="onEnter"
      />
      <a-space style="margin-top: 8px">
        <a-button
          type="primary"
          :loading="streaming"
          :disabled="streaming || !input.trim()"
          @click="send"
          >{{
            streaming ? $t('order.assistant.thinking') : $t('order.assistant.send')
          }}</a-button
        >
        <a-button v-if="streaming" danger @click="stop">{{
          $t('order.assistant.stop')
        }}</a-button>
        <a-button :disabled="!messages.length || streaming" @click="clear">{{
          $t('order.assistant.clear')
        }}</a-button>
      </a-space>
    </div>
  </div>
</template>

<script lang="ts" setup>
  import { nextTick, onUnmounted, ref } from 'vue';
  import { useI18n } from 'vue-i18n';
  import Vditor from 'vditor';
  import 'vditor/dist/index.css';
  import { chatStream, ChatMessage } from '@/apis/chat';
  import { sanitizeHTML } from '@/lib/sanitize';

  interface AssistantContext {
    source?: string;
    dataBase?: string;
    tables?: string[];
    sql?: string;
  }

  interface Msg {
    role: 'user' | 'assistant';
    content: string;
    html?: string;
    streaming?: boolean;
  }

  const props = defineProps<{ context?: AssistantContext }>();

  const { t } = useI18n();

  const messages = ref<Msg[]>([]);
  const input = ref('');
  const streaming = ref(false);
  const error = ref('');
  const listRef = ref<HTMLElement>();

  let controller: AbortController | null = null;

  // 把页面上选中的数据源/库/表与编辑器里的 SQL 作为上下文附加到当前提问，
  // 这样可以直接问「帮我优化这段 SQL」。上下文只在请求里发送，不进入界面展示。
  const buildContext = () => {
    const c = props.context || {};
    const parts: string[] = [];
    if (c.source) parts.push(`数据源: ${c.source}`);
    if (c.dataBase) parts.push(`目标库: ${c.dataBase}`);
    if (c.tables?.length) parts.push(`相关表: ${c.tables.join(', ')}`);
    if (c.sql) parts.push(`SQL:\n${c.sql}`);
    return parts.length ? `[当前上下文]\n${parts.join('\n')}\n\n` : '';
  };

  const scrollToBottom = () =>
    nextTick(() => {
      if (listRef.value) listRef.value.scrollTop = listRef.value.scrollHeight;
    });

  const send = async () => {
    const text = input.value.trim();
    if (!text || streaming.value) return;
    error.value = '';
    input.value = '';
    messages.value.push({ role: 'user', content: text });
    messages.value.push({ role: 'assistant', content: '', streaming: true });
    // 取回数组里的响应式代理，后续增量写入才能触发渲染
    const reply = messages.value[messages.value.length - 1];

    const history: ChatMessage[] = messages.value
      .filter((m) => !m.streaming)
      .map((m) => ({ role: m.role, content: m.content }));
    const last = history[history.length - 1];
    if (last && last.role === 'user') last.content = buildContext() + last.content;

    streaming.value = true;
    controller = new AbortController();
    scrollToBottom();
    try {
      await chatStream(
        history,
        (delta) => {
          reply.content += delta;
          scrollToBottom();
        },
        controller.signal
      );
      if (reply.content) {
        // AI 输出视为不可信内容（提示词注入风险）：先脱离 DOM 渲染再清洗后挂载。
        // Vditor.preview 是异步的（返回 Promise），必须 await 后再读 innerHTML，否则拿到空串
        const holder = document.createElement('div');
        await Vditor.preview(holder, reply.content);
        reply.html = sanitizeHTML(holder.innerHTML);
      }
    } catch (e) {
      const err = e as Error;
      // 用户主动停止不算失败
      if (err.name === 'AbortError') {
        if (!reply.content) messages.value.pop();
      } else {
        error.value = err.message || t('order.assistant.failed');
        if (!reply.content) messages.value.pop();
      }
    } finally {
      reply.streaming = false;
      streaming.value = false;
      controller = null;
      scrollToBottom();
    }
  };

  const stop = () => controller?.abort();

  const clear = () => {
    messages.value = [];
    error.value = '';
  };

  const onEnter = (e: KeyboardEvent) => {
    if (e.shiftKey) return; // Shift+Enter 换行
    e.preventDefault();
    send();
  };

  onUnmounted(() => controller?.abort());
</script>

<style scoped>
  .assistant-list {
    height: 420px;
    padding: 12px;
    overflow: auto;
    border: 1px solid rgba(128, 128, 128, 0.2);
    border-radius: 8px;
  }
  .assistant-empty {
    padding: 40px 12px;
    color: inherit;
    text-align: center;
    opacity: 0.45;
  }
  .assistant-row {
    display: flex;
    margin-bottom: 12px;
  }
  .assistant-row.user {
    justify-content: flex-end;
  }
  .assistant-bubble {
    max-width: 80%;
    padding: 8px 12px;
    white-space: pre-wrap;
    word-break: break-word;
    border-radius: 8px;
    background: rgba(128, 128, 128, 0.12);
  }
  .assistant-row.user .assistant-bubble {
    background: rgba(82, 117, 144, 0.18);
  }
  .assistant-bubble.md {
    white-space: normal;
  }
  .assistant-bubble.md :deep(pre) {
    padding: 8px;
    overflow: auto;
    background: rgba(128, 128, 128, 0.12);
    border-radius: 6px;
  }
  /* 生成中的光标 */
  .assistant-caret {
    display: inline-block;
    width: 6px;
    height: 14px;
    margin-left: 2px;
    vertical-align: text-bottom;
    background: currentColor;
    opacity: 0.6;
    animation: assistant-blink 1s step-end infinite;
  }
  @keyframes assistant-blink {
    50% {
      opacity: 0;
    }
  }
</style>
