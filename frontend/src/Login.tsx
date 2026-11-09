import { useState } from 'react';
import { ClockCircleOutlined, LinkOutlined, MessageOutlined, LockOutlined, UserOutlined } from '@ant-design/icons';
import { Button, Form, Input } from 'antd';
import { api, errorText } from './api';
import { Brand, ContinueButton, ErrorNotice, Footer } from './components';

export const Login = ({ name, onLogin, activationToken = '', onCancelActivation }: { name: string; onLogin: () => Promise<void>; activationToken?: string; onCancelActivation?: () => void }) => {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [register, setRegister] = useState(false);
  const inviting = register || !!activationToken;
  const submit = async (v: { username: string; password: string }) => {
    setBusy(true); setError('');
    try { await api('login', v); await onLogin(); } catch (e) { setError(errorText(e)); } finally { setBusy(false); }
  };
  const activate = async ({ token }: { token: string }) => {
    setBusy(true); setError('');
    try { await api('activate', { token: token.trim() }); await onLogin(); }
    catch (e) { setError(errorText(e)); } finally { setBusy(false); }
  };
  return <main className="login-shell">
    <section className="login-story"><Brand name={name} /><div className="story-content"><h1>你的连接，<br />一目了然。</h1><p>查看剩余时间，管理订阅，<br />让每一次连接都心中有数。</p><ul className="feature-lines"><li><ClockCircleOutlined /><span>到期时间与用量，清楚可见</span></li><li><LinkOutlined /><span>专属订阅，随时获取</span></li><li><MessageOutlined /><span>需要帮助，直接联系站长</span></li></ul></div><p className="story-bottom">把服务留给面板，把便利带给你。</p></section>
    <section className="login-form-side"><div className="login-form-wrap"><h2>{inviting ? '用邀请开启账户' : '欢迎回来'}</h2><p className="lead">{inviting ? '输入站长发给你的专属邀请码，自动关联账户，再设置登录密码。' : '登录你的账户，查看订阅与使用情况。'}</p><ErrorNotice error={error} />
      {inviting ? <Form key={activationToken || 'invitation'} layout="vertical" requiredMark={false} disabled={busy} initialValues={{ token: activationToken }} onFinish={(v: { token: string }) => void activate(v)}>
        <Form.Item label="邀请码" name="token" rules={[{ required: true, whitespace: true, message: '请输入站长提供的邀请码' }]}><Input.TextArea rows={3} placeholder="粘贴专属邀请码" autoComplete="off" autoFocus maxLength={128} /></Form.Item>
        <ContinueButton htmlType="submit" block loading={busy}>验证邀请码并继续</ContinueButton>
      </Form> :
      <Form layout="vertical" requiredMark={false} disabled={busy} onFinish={(v: { username: string; password: string }) => void submit(v)}>
        <Form.Item label="账号" name="username" rules={[{ required: true, message: '请输入账号' }]}><Input prefix={<UserOutlined />} placeholder="3x-ui 客户端名称或管理员账号" autoComplete="username" maxLength={128} autoFocus /></Form.Item>
        <Form.Item label="密码" name="password" rules={[{ required: true, message: '请输入密码' }]}><Input.Password prefix={<LockOutlined />} placeholder="请输入密码" autoComplete="current-password" maxLength={72} /></Form.Item>
        <ContinueButton htmlType="submit" block loading={busy}>登录账户</ContinueButton>
      </Form>}
      <Button type="link" disabled={busy} onClick={() => { setError(''); setRegister(!inviting); if (inviting) onCancelActivation?.(); }}>{inviting ? '已有账户，返回登录' : '首次使用？邀请码注册'}</Button>
      <p className="login-help">邀请码仅用于首次注册，24 小时内有效且只能使用一次。失效或无法登录时，请联系站长。</p><Footer /></div></section>
  </main>;
};
