import { useState } from 'react';
import { ClockCircleOutlined, LinkOutlined, MessageOutlined, LockOutlined, UserOutlined } from '@ant-design/icons';
import { Button, Form, Input } from 'antd';
import { api, errorText } from './api';
import { Brand, ContinueButton, ErrorNotice, Footer } from './components';

export const Login = ({ name, onLogin, activationToken = '', onCancelActivation }: { name: string; onLogin: () => Promise<void>; activationToken?: string; onCancelActivation?: () => void }) => {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const submit = async (v: { username: string; password: string }) => {
    setBusy(true); setError('');
    try { await api('login', v); await onLogin(); } catch (e) { setError(errorText(e)); } finally { setBusy(false); }
  };
  const activate = async () => {
    setBusy(true); setError('');
    try { await api('activate', { token: activationToken }); await onLogin(); }
    catch (e) { setError(errorText(e)); } finally { setBusy(false); }
  };
  return <main className="login-shell">
    <section className="login-story"><Brand name={name} /><div className="story-content"><h1>你的连接，<br />一目了然。</h1><p>查看剩余时间，管理订阅，<br />让每一次连接都心中有数。</p><ul className="feature-lines"><li><ClockCircleOutlined /><span>到期时间与用量，清楚可见</span></li><li><LinkOutlined /><span>专属订阅，随时获取</span></li><li><MessageOutlined /><span>需要帮助，直接联系站长</span></li></ul></div><p className="story-bottom">把服务留给面板，把便利带给你。</p></section>
    <section className="login-form-side"><div className="login-form-wrap"><h2>{activationToken ? '欢迎，开启你的账户' : '欢迎回来'}</h2><p className="lead">{activationToken ? '首次无需密码。验证邀请后，设置你自己的本站登录密码。' : '登录你的账户，查看订阅与使用情况。'}</p><ErrorNotice error={error} />
      {activationToken ? <><ContinueButton block loading={busy} onClick={() => void activate()}>验证邀请并继续</ContinueButton><Button type="link" disabled={busy} onClick={onCancelActivation}>已有密码，返回登录</Button></> :
      <Form layout="vertical" requiredMark={false} onFinish={(v: { username: string; password: string }) => void submit(v)}>
        <Form.Item label="账号" name="username" rules={[{ required: true, message: '请输入账号' }]}><Input prefix={<UserOutlined />} placeholder="请输入账号" autoComplete="username" maxLength={32} autoFocus /></Form.Item>
        <Form.Item label="密码" name="password" rules={[{ required: true, message: '请输入密码' }]}><Input.Password prefix={<LockOutlined />} placeholder="请输入密码" autoComplete="current-password" maxLength={72} /></Form.Item>
        <ContinueButton htmlType="submit" block loading={busy}>登录账户</ContinueButton>
      </Form>}<p className="login-help">首次使用请打开站长发送的激活链接。链接失效或无法登录时，请联系站长。</p><Footer /></div></section>
  </main>;
};
