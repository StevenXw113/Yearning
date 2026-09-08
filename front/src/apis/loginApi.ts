import { request } from '@/config/request';

export interface LoginFrom {
  username: string;
  password: string;
  is_oidc: boolean;
}

export function signIn(login: LoginFrom) {
  // LDAP 已从后端彻底移除，仅保留本地账号登录
  return request.post('/login', login);
}

export function systemRegisterState() {
  return request.get('/fetch');
}

export function systemLang() {
  return request.get('/lang');
}

export function getOIDCState() {
  return request.get('/oidc/state');
}
