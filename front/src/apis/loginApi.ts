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

// MFA 登录第二步：携带第一步返回的 ticket + 认证器动态码换取正式 JWT
export function mfaSignIn(ticket: string, code: string) {
  return request.post('/login/mfa', { ticket, code });
}
