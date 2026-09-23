import { COMMON_URI } from '@/config/request';
import { store } from '@/store';

export interface ChatMessage {
  role: 'user' | 'assistant' | 'system';
  content: string;
}

/** 流结束标记，与后端 chatFrame/sseDone 对应 */
export const SSE_DONE = '[DONE]';

/**
 * 从缓冲区里切出所有完整 SSE 帧，返回剩余的不完整尾部。
 * 帧以空行分隔（后端每帧形如 `data: {"v":"..."}\n\n`）。
 */
export const takeSSEFrames = (
  buffer: string
): { frames: string[]; rest: string } => {
  const frames: string[] = [];
  let rest = buffer;
  let idx: number;
  while ((idx = rest.indexOf('\n\n')) >= 0) {
    frames.push(rest.slice(0, idx));
    rest = rest.slice(idx + 2);
  }
  return { frames, rest };
};

/**
 * 调用后端流式对话接口（POST /api/v2/chat，text/event-stream）。
 * 用 fetch 而非 axios：浏览器端 axios 拿不到流式响应体。
 * 后端在「未配置 AI / 上游拒绝」时会返回普通 JSON（非流式），此处据此抛出明确错误。
 */
export async function chatStream(
  messages: ChatMessage[],
  onDelta: (text: string) => void,
  signal?: AbortSignal
): Promise<void> {
  const token = store.state.user?.account?.token;
  const res = await fetch(`${COMMON_URI}/chat`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify({ messages }),
    signal,
  });

  if (!(res.headers.get('content-type') || '').includes('text/event-stream')) {
    const data = await res.json().catch(() => null);
    throw new Error(data?.text || `AI 服务异常 (${res.status})`);
  }
  if (!res.body) {
    throw new Error('AI 服务未返回内容');
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    // stream: true 让跨 chunk 被切断的多字节字符能正确拼接
    buffer += decoder.decode(value, { stream: true });
    const { frames, rest } = takeSSEFrames(buffer);
    buffer = rest;
    for (const frame of frames) {
      if (!frame.startsWith('data: ')) continue;
      const payload = frame.slice(6);
      if (payload === SSE_DONE) return;
      let body: { v?: string; error?: string };
      try {
        body = JSON.parse(payload);
      } catch {
        continue;
      }
      if (body.error) throw new Error(body.error);
      if (body.v) onDelta(body.v);
    }
  }
}
