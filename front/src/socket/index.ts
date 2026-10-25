import { useIntervalFn } from '@vueuse/core';

// ws 保活配置(@vueuse 12+ 移除 heartbeat.interval,改 scheduler 注入)。
// pongTimeout 默认 1s 无回包即断线重连,而后端不回显 ping——旧版语义是
// 「仅保活不判死」,因此关掉判死,保留 5s 一次的 keep-alive(穿透代理空闲超时)。
export function wsHeartbeat() {
  return {
    scheduler: (cb: () => void) => useIntervalFn(cb, 5000, { immediate: false }),
    pongTimeout: Number.MAX_SAFE_INTEGER,
    message: 'ping',
  };
}
