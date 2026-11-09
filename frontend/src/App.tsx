import { useCallback, useEffect, useState } from 'react';
import { App, Button, Form, Input, Skeleton } from 'antd';
import { DashboardOutlined, LogoutOutlined, SettingOutlined, TeamOutlined, UserOutlined } from '@ant-design/icons';
import { api, ApiError, errorText, setCSRF, type Session } from './api';
import { Brand, ErrorNotice, Panel } from './components';
import { Setup } from './Setup';
import { Login } from './Login';
import { Dashboard } from './Dashboard';
import { AdminUsers, SiteSettings } from './Admin';
import { passwordRule } from './validation';
import { FirstPassword } from './FirstPassword';
import { Certificates } from './Certificates';

const PasswordForm = ({ onChanged }: { onChanged: () => void }) => {
  const [busy, setBusy] = useState(false); const [error, setError] = useState('');
  const { message } = App.useApp();
  const submit = async (v: { current: string; password: string }) => {
    setBusy(true); setError('');
    try { await api('password', { current: v.current, password: v.password }); message.success('密码已修改，请重新登录'); onChanged(); }
    catch (e) { setError(errorText(e)); } finally { setBusy(false); }
  };
  return <Panel title="修改登录密码" className="narrow-panel"><p className="muted">修改成功后，所有已登录会话将退出。</p><ErrorNotice error={error} /><Form layout="vertical" requiredMark={false} onFinish={(v) => void submit(v)}>
    <Form.Item name="current" label="当前密码" rules={[{ required: true, message: '请输入当前密码' }]}><Input.Password autoComplete="current-password" /></Form.Item>
    <Form.Item name="password" label="新密码" rules={[passwordRule]}><Input.Password autoComplete="new-password" /></Form.Item>
    <Form.Item name="confirm" label="确认新密码" dependencies={['password']} rules={[{ required: true, message: '请确认新密码' }, ({ getFieldValue }) => ({ validator(_, value) { return value === getFieldValue('password') ? Promise.resolve() : Promise.reject(new Error('两次密码不一致')); } })]}><Input.Password autoComplete="new-password" /></Form.Item>
    <Button type="primary" htmlType="submit" loading={busy}>保存新密码</Button>
  </Form></Panel>;
};

export const PortalApp = () => {
  const [activationToken, setActivationToken] = useState(() => new URLSearchParams(location.hash.slice(1)).get('activate') || '');
  const [boot, setBoot] = useState<{ mode: string; name: string }>();
  const [session, setSession] = useState<Session>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [tab, setTab] = useState('home');
  const [loggingOut, setLoggingOut] = useState(false);
  const { message } = App.useApp();
  const clear = useCallback(() => { setCSRF(''); setSession(undefined); setTab('home'); }, []);
  useEffect(() => {
    // 已打开登录页时点击同路径邀请只触发 hashchange，不会重新挂载应用。
    const captureInvitation = () => {
      const token = new URLSearchParams(location.hash.slice(1)).get('activate');
      if (token) { clear(); setActivationToken(token); history.replaceState(null, '', location.pathname + location.search); }
    };
    window.addEventListener('hashchange', captureInvitation);
    return () => window.removeEventListener('hashchange', captureInvitation);
  }, [clear]);
  const refresh = useCallback(async () => {
    const s = await api<Session>('me'); setCSRF(s.csrf); setSession(s); setTab('home');
  }, []);
  useEffect(() => {
    let live = true;
    if (location.hash) history.replaceState(null, '', location.pathname + location.search);
    const start = async () => {
      try {
        const b = await api<{ mode: string; name: string }>('bootstrap'); if (!live) return; setBoot(b); document.title = `${b.name} · 用户中心`;
        if (b.mode !== 'setup' && !activationToken) { try { const s = await api<Session>('me'); if (live) { setCSRF(s.csrf); setSession(s); } } catch (e) { if (!(e instanceof ApiError && e.status === 401)) throw e; } }
      } catch (e) { if (live) setError(errorText(e)); } finally { if (live) setLoading(false); }
    };
    void start(); window.addEventListener('portal:unauthorized', clear);
    return () => { live = false; window.removeEventListener('portal:unauthorized', clear); };
  }, [clear]);
  const logout = async () => {
    setLoggingOut(true);
    try { await api('logout', {}); clear(); } catch (e) { message.error(errorText(e)); } finally { setLoggingOut(false); }
  };
  if (loading) return <div className="boot-state"><Brand /><Skeleton active paragraph={{ rows: 4 }} /></div>;
  if (error || !boot) return <div className="boot-state"><ErrorNotice error={error || '无法读取站点信息'} /><Button onClick={() => location.reload()}>重新加载</Button></div>;
  if (boot.mode === 'setup') return <Setup />;
  if (!session) return <Login name={boot.name} activationToken={activationToken} onCancelActivation={() => setActivationToken('')} onLogin={async () => { await refresh(); setActivationToken(''); }} />;
  if (session.user.mustSetPassword) return <FirstPassword name={session.name} username={session.user.username} onDone={refresh} onLogout={logout} />;
  const admin = session.user.role === 'admin';
  const navigation = [{ key: 'home', title: admin ? '用户管理' : '订阅概览', icon: admin ? <TeamOutlined aria-hidden="true" /> : <DashboardOutlined aria-hidden="true" /> }, ...(admin ? [{ key: 'settings', title: '站点设置', icon: <SettingOutlined aria-hidden="true" /> }] : []), { key: 'account', title: '账户安全', icon: <UserOutlined aria-hidden="true" /> }];
  return <div className="workspace"><aside className="sidebar"><Brand name={session.name} /><div className="workspace-label">{admin ? '站长工作区' : '我的用户中心'}</div><nav aria-label="主要导航">{navigation.map((item) => <button key={item.key} className={`nav-item ${tab === item.key ? 'selected' : ''}`} aria-current={tab === item.key ? 'page' : undefined} onClick={() => setTab(item.key)}>{item.icon}<span>{item.title}</span></button>)}</nav><div className="sidebar-bottom"><span className="account-avatar"><UserOutlined aria-hidden="true" /></span><div><strong>{session.user.username}</strong><span>{admin ? '管理员' : '普通用户'}</span></div><Button aria-label="退出登录" type="text" icon={<LogoutOutlined />} loading={loggingOut} onClick={() => void logout()} /></div></aside>
    <main className="main-content"><header className="page-heading"><div><h1>{navigation.find((n) => n.key === tab)?.title}</h1><p>{tab === 'home' ? (admin ? '邀请已有面板用户，自动关联账户。' : '你的订阅、用量和剩余时间，都在这里。') : tab === 'settings' ? '管理连接方式与用户端服务。' : '保护你的账户，从一个可靠的密码开始。'}</p></div><span className="workspace-badge">{session.name}</span></header>
      {tab === 'home' && (admin ? <AdminUsers /> : <Dashboard supportUrl={session.supportUrl} />)}
      {tab === 'settings' && admin && <><SiteSettings onSaved={(name) => { setSession({ ...session, name }); setBoot({ ...boot, name }); }} /><Certificates /></>}
      {tab === 'account' && <PasswordForm onChanged={clear} />}
      <footer className="workspace-footer">{session.name}<span>服务信息以面板最近同步结果为准</span></footer>
    </main>
  </div>;
};
