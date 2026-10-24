// antd App 上下文桥：供 axios 拦截器等**非组件上下文**使用——
// 静态 message/notification 在 antd 4 的 CSS-in-JS 主题下不吃 ConfigProvider
// 的算法，浮层会与主题脱节。桥里的实例与 <a-app> 同源，主题自动一致。
//
// 绑定时机：由 <a-app> **内部**的桥接组件（antdAppBridge.vue）挂载时调用
// bindAntdFeedback——useApp() 必须在 a-app 的子组件里调用才拿得到上下文实例
// （在提供者 App.vue 自身的 setup 里调用会得到缺方法的占位对象，实机验证踩过）。
// App 挂载前的极早期错误兜底走原生静态调用。
//
// 类型按消费者实际用到的最小形状声明：静态 API 与 useApp 实例的函数签名
// 在细节参数上有差异（duration 联合类型等），直接抄实例类型会互相赋不上。
import { message as staticMessage, notification as staticNotification } from 'ant-design-vue';

interface FeedbackBridge {
  message: {
    success(content: unknown): void;
    error(content: unknown): void;
    info(content: unknown): void;
    warning(content: unknown): void;
    loading(content: unknown): void;
  };
  notification: {
    success(args: unknown): void;
    error(args: unknown): void;
    info(args: unknown): void;
    warning(args: unknown): void;
  };
}

export const antdFeedback: FeedbackBridge = {
  message: staticMessage,
  notification: staticNotification,
};

export function bindAntdFeedback(instances: FeedbackBridge) {
  antdFeedback.message = instances.message;
  antdFeedback.notification = instances.notification;

  // 诊断辅助：浏览器控制台可直接调用桥实例
  if (typeof window !== 'undefined') {
    (window as any).__antdFeedback = antdFeedback;
  }
}
