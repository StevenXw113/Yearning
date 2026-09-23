import i18n from '@/lang';
import { OrderState, QueryState } from '@/types';
import {
  CheckCircleOutlined,
  SyncOutlined,
  CloseCircleOutlined,
} from '@ant-design/icons-vue';
import mitt from 'mitt';
// @ts-ignore
const { t } = i18n.global;

export const StateUsage = (state: number) => {
  switch (state) {
    case OrderState.PROCESS:
      return {
        color: '#408B9B',
        title: t('order.state.process'),
        icon: SyncOutlined,
        isEnd: false,
      };
    case OrderState.WAIT:
      return {
        color: '#408B9B',
        title: t('order.state.wait'),
        icon: SyncOutlined,
        isEnd: false,
      };
    case OrderState.AUDIT:
      return {
        color: '#408B9B',
        title: t('order.state.audit'),
        icon: SyncOutlined,
        isEnd: false,
      };
    case OrderState.SUCCESS:
      return {
        color: '#43A687',
        title: t('order.state.success'),
        icon: CheckCircleOutlined,
        isEnd: true,
      };
    case OrderState.REJECT:
      return {
        color: '#EA495F',
        title: t('order.state.reject'),
        icon: CloseCircleOutlined,
        isEnd: true,
      };
    case OrderState.ERROR:
      return {
        color: '#EA495F',
        title: t('order.state.error'),
        icon: CloseCircleOutlined,
        isEnd: true,
      };
    case OrderState.Undo:
      return {
        color: '#EA495F',
        title: t('order.undo'),
        icon: CloseCircleOutlined,
        isEnd: true,
      };
    case OrderState.Terminate:
      return {
        color: '#EA495F',
        title: t('order.terminate'),
        icon: CloseCircleOutlined,
        isEnd: true,
      };
    default:
      return {};
  }
};

export const StateQueryUsage = (state: number) => {
  switch (state) {
    case QueryState.AUDIT:
      return {
        color: '#408B9B',
        title: t('order.state.audit'),
        icon: SyncOutlined,
      };
    case QueryState.PROCESS:
      return {
        color: '#408B9B',
        title: t('order.query.audit.state.process'),
        icon: SyncOutlined,
      };
    case QueryState.DONE:
      return {
        color: '#EA495F',
        title: t('order.query.audit.state.done'),
        icon: CloseCircleOutlined,
      };
    case QueryState.REJECT:
      return {
        color: '#EA495F',
        title: t('order.state.reject'),
        icon: CloseCircleOutlined,
      };
    default:
      return {};
  }
};

// ---- SQL 文件上传校验 ----
// accept 属性只影响文件选择器的默认过滤（可被拖拽/改名绕过），因此实际校验放在读取阶段。
// 抛出的 Error.message 已是当前语言的提示文案，调用方直接展示即可。
const SQL_FILE_EXT = ['.sql', '.txt'];
const MAX_SQL_FILE_SIZE = 10 * 1024 * 1024;

export const readSQLFile = async (file: File): Promise<string> => {
  if (!SQL_FILE_EXT.some((ext) => file.name.toLowerCase().endsWith(ext))) {
    throw new Error(t('order.apply.upload.type'));
  }
  if (file.size > MAX_SQL_FILE_SIZE) {
    throw new Error(t('order.apply.upload.tooLarge'));
  }
  let text: string;
  try {
    text = await file.text();
  } catch {
    throw new Error(t('order.apply.upload.failed'));
  }
  // 文本文件不应含 NUL 字节：改名伪装成 .sql 的二进制文件在这里被拦下
  if (text.includes('\u0000')) {
    throw new Error(t('order.apply.upload.binary'));
  }
  return text;
};

export const checkSchema = () => {
  let baseURL = '127.0.0.1:8000';
  let scheme = 'ws://';
  if (import.meta.env.MODE !== 'dev') {
    baseURL = document.location.host;
  }
  if (document.location.protocol === 'https:') {
    scheme = 'wss://';
  }
  return `${scheme}${baseURL}`;
};

export const EventBus = mitt();
