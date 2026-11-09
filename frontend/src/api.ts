export class ApiError extends Error {
  constructor(message: string, public status: number) { super(message); }
}

let csrf = '';
export const setCSRF = (value: string) => { csrf = value; };

// 路径始终相对当前秘密入口，不从本地存储恢复账号凭据。
export const api = async <T>(path: string, body?: unknown, signal?: AbortSignal): Promise<T> => {
  const response = await fetch(new URL(`api/${path}`, window.location.href), {
    method: body === undefined ? 'GET' : 'POST',
    credentials: 'same-origin', cache: 'no-store', signal,
    headers: body === undefined ? {} : { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const result = await response.json().catch(() => ({}));
  if (!response.ok) {
    if (response.status === 401 && path !== 'login' && path !== 'activate') window.dispatchEvent(new Event('portal:unauthorized'));
    throw new ApiError(typeof result.error === 'string' ? result.error : '请求未完成，请刷新后重试', response.status);
  }
  return result as T;
};

export const errorText = (e: unknown) => e instanceof Error ? e.message : '操作未完成，请稍后重试';
export type User = { id: number; username: string; role: 'admin' | 'user'; disabled: boolean; mustSetPassword: boolean; clientEmail?: string; createdAt: number };
export type Session = { user: User; csrf: string; name: string; supportUrl: string };
export type Summary = {
  clientName: string; status: string; expiryTime: number; upload: number; download: number;
  total: number; serverTime: number; links: string[]; linkError: string;
  canRotate: boolean; rotateReason: string; supportUrl: string;
  routes: { primary: string; backup: string; reason: string }[];
};
export type Settings = { name: string; panelUrl: string; supportUrl: string; allowRotation: boolean; relayOrigins: string };
export type Invitation = { activationUrl: string; expiresAt: number; username: string };
