import axios, { AxiosInstance } from 'axios';
import { notification } from 'ant-design-vue';
import { store } from '@/store';
import router from '@/router';
import i18n from '@/lang';

interface Res<T> {
  code: number;
  text: string;
  payload: T;
}

const { t } = i18n.global;

const COMMON_URI = '/api/v2';

const request: AxiosInstance = axios.create({
  timeout: 200000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// 会话失效后统一清理本地凭据，避免残留的失效 token 导致页面反复跳登录/死循环
const resetSession = () => {
  store.commit('user/USER_STORE', {
    token: '',
    real_name: '',
    user: '',
    rule: '',
    is_record: 2,
  });
  sessionStorage.clear();
};

const errorHandler = (error: {
  response: { data: { message: string }; status: number };
}) => {
  if (error.response) {
    if (error.response.status === 401) {
      // 若仍持有失效凭据，先清理，避免“跳登录后守卫仍放行 → 再次 401”的死循环
      if (store.state.user?.account?.token) {
        resetSession();
      }
      notification.error({
        message: t('common.session.title'),
        description: t('common.session.desc'),
      });
      if (router.currentRoute.value.name !== 'login') {
        router.replace('/login');
      }
      return Promise.reject(error);
    }
    const data = error.response.data;
    notification.error({
      message: t('common.session.state') + `:${error.response.status}`,
      description: data.message,
    });
  }
  return Promise.reject(error);
};

const responseInject = (res: Res<never>) => {
  if (res.text !== '' && res.code === 1200) {
    notification.info({
      message: t('common.session.state') + ':1200',
      description: res.text,
    });
  }

  if (res.code > 1200) {
    notification.error({
      message: t('common.session.state') + `:${res.code}`,
      description: res.text,
    });
  }
};

request.interceptors.request.use((config) => {
  // 每个请求都从 store 实时取 token，避免模块加载时快照到失效/空凭据
  const token = store.state.user?.account?.token;
  if (token) {
    config.headers.set('Authorization', 'Bearer ' + token);
    // 供同源内嵌页（如 AI 聊天 iframe）读取，避免把 JWT 放进 URL
    sessionStorage.setItem('yrn_jwt', token);
  }
  return config;
}, errorHandler);

request.interceptors.response.use((response) => {
  responseInject(response.data);
  return response;
}, errorHandler);

export { request, COMMON_URI, Res };
