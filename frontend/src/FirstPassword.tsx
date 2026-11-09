import { useState } from 'react';
import { App, Button, Form, Input } from 'antd';
import { api, errorText } from './api';
import { Brand, ContinueButton, ErrorNotice, Footer } from './components';
import { passwordRule } from './validation';

export const FirstPassword = ({ name, username, onDone, onLogout }: { name: string; username: string; onDone: () => void; onLogout: () => Promise<void> }) => {
  const [busy, setBusy] = useState(false), [error, setError] = useState('');
  const { message } = App.useApp();
  const submit = async ({ password }: { password: string }) => {
    setBusy(true); setError('');
    try { await api('first-password', { password }); message.success('密码已设置，请使用新密码登录'); onDone(); }
    catch (e) { setError(errorText(e)); } finally { setBusy(false); }
  };
  return <main className="activation-shell"><div className="activation-form"><Brand name={name} /><h1>最后一步，设置密码</h1><p className="lead">你的账号是 <strong>{username}</strong>。设置完成后，即可登录查看订阅。密码仅保存在本站。</p><ErrorNotice error={error} />
    <Form layout="vertical" requiredMark={false} onFinish={(values: { password: string }) => void submit(values)}>
      <Form.Item name="password" label="设置登录密码" rules={[passwordRule]}><Input.Password autoComplete="new-password" autoFocus placeholder="输入你的登录密码" /></Form.Item>
      <Form.Item name="confirm" label="确认密码" dependencies={['password']} rules={[{ required: true, message: '请再次输入密码' }, ({ getFieldValue }) => ({ validator(_, value) { return value === getFieldValue('password') ? Promise.resolve() : Promise.reject(new Error('两次密码不一致')); } })]}><Input.Password autoComplete="new-password" /></Form.Item>
      <ContinueButton htmlType="submit" block loading={busy}>设置密码，前往登录</ContinueButton>
    </Form><p className="login-help">请在 10 分钟内完成；过期后需联系站长重新发送激活链接。</p><Button disabled={busy} onClick={() => void onLogout()}>退出</Button><Footer />
  </div></main>;
};
