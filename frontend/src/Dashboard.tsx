import { useCallback, useEffect, useRef, useState } from 'react';
import { App, Button, Empty, Input, Modal, Skeleton, Tag } from 'antd';
import { CopyOutlined, ExportOutlined, MessageOutlined, ReloadOutlined, SyncOutlined } from '@ant-design/icons';
import { api, errorText, type Summary } from './api';
import { ErrorNotice, Panel } from './components';

const bytes = (n: number) => {
  if (!Number.isFinite(n) || n < 0) return '—';
  if (n < 1024) return `${n} B`;
  if (n < 1024 ** 2) return `${(n / 1024).toFixed(2)} KB`;
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(2)} MB`;
  return `${(n / 1024 ** 3).toFixed(2)} GB`;
};
const remaining = (expiry: number, now: number) => {
  if (expiry === 0) return '不限期';
  if (expiry < 0) return `首次使用后有效 ${Math.ceil(Math.abs(expiry) / 86400000)} 天`;
  const seconds = Math.max(0, Math.floor((expiry - now) / 1000));
  if (!seconds) return '已到期';
  const d = Math.floor(seconds / 86400), h = Math.floor(seconds / 3600) % 24, m = Math.floor(seconds / 60) % 60;
  return `${d > 0 ? `${d} 天 ` : ''}${h} 时 ${m} 分 ${seconds % 60} 秒`;
};
const labels: Record<string, { label: string; color: string }> = {
  active: { label: '服务正常', color: 'green' }, unlimited: { label: '不限期', color: 'green' },
  expired: { label: '已到期', color: 'red' }, depleted: { label: '流量已用尽', color: 'orange' },
  disabled: { label: '已停用', color: 'default' }, pending: { label: '待首次使用', color: 'blue' },
};

export const Dashboard = ({ supportUrl }: { supportUrl: string }) => {
  const [data, setData] = useState<Summary>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [clock, setClock] = useState(Date.now());
  const [received, setReceived] = useState(0);
  const mounted = useRef(true), inFlight = useRef(false), request = useRef<AbortController | undefined>(undefined);
  const anchor = useRef({ server: Date.now(), at: performance.now() });
  const [resetOpen, setResetOpen] = useState(false), [password, setPassword] = useState('');
  const [resetBusy, setResetBusy] = useState(false), [resetError, setResetError] = useState('');
  const { message } = App.useApp();
  const refresh = useCallback(async () => {
    if (inFlight.current) return;
    inFlight.current = true; setBusy(true);
    const ctrl = new AbortController(); request.current = ctrl;
    try {
      const value = await api<Summary>('summary', undefined, ctrl.signal);
      if (mounted.current && !ctrl.signal.aborted) { setData(value); setReceived(Date.now()); setError(''); anchor.current = { server: value.serverTime, at: performance.now() }; setClock(value.serverTime); }
    } catch (e) { if (mounted.current && !ctrl.signal.aborted) setError(errorText(e)); }
    finally { if (request.current === ctrl) { inFlight.current = false; if (mounted.current) setBusy(false); } }
  }, []);
  useEffect(() => {
    mounted.current = true; void refresh();
    const poll = window.setInterval(() => { if (!document.hidden) void refresh(); }, 15000);
    const tick = window.setInterval(() => setClock(anchor.current.server + performance.now() - anchor.current.at), 1000);
    const focus = () => { if (!document.hidden) void refresh(); };
    document.addEventListener('visibilitychange', focus);
    return () => { mounted.current = false; request.current?.abort(); inFlight.current = false; clearInterval(poll); clearInterval(tick); document.removeEventListener('visibilitychange', focus); };
  }, [refresh]);
  const rotate = async () => {
    setResetBusy(true); setResetError('');
    try { const result = await api<{ message: string }>('rotate', { password }); message.success(result.message); setResetOpen(false); setPassword(''); await refresh(); }
    catch (e) { setResetError(errorText(e)); await refresh(); } finally { setResetBusy(false); }
  };
  const copy = (link: string) => { void navigator.clipboard.writeText(link).then(() => message.success('订阅地址已复制')).catch(() => message.error('复制失败，请选中地址手动复制')); };
  const used = data ? data.upload + data.download : 0;
  const percent = data?.total ? Math.min(100, used / data.total * 100) : 0;
  const state = data ? labels[data.status] || labels.active : labels.active;
  return <>
    <div className="sync-bar"><span><span className={`status-dot ${error ? 'status-dot-error' : ''}`} />{received ? `最近同步 ${new Date(received).toLocaleTimeString('zh-CN')}${error ? ' · 当前数据可能已过时' : ''}` : '正在获取服务状态'}</span><Button type="text" size="small" icon={<SyncOutlined spin={busy} />} disabled={busy} onClick={() => void refresh()}>刷新</Button></div>
    <ErrorNotice error={error} />
    {!data ? <Panel title="服务信息">{busy ? <Skeleton active paragraph={{ rows: 5 }} /> : <Empty description="暂时无法获取信息，请检查连接后刷新" />}</Panel> : <>
      <div className="overview-grid"><Panel title="我的服务" action={<Tag color={state.color}>{state.label}</Tag>}>
        <div className="service-identity"><span className="service-symbol"><ExportOutlined /></span><div><strong>{data.clientName}</strong><p>你的专属订阅账户</p></div></div>
        <dl className="service-details"><div><dt>剩余时间</dt><dd className="countdown">{remaining(data.expiryTime, clock)}</dd></div><div><dt>到期时间</dt><dd>{data.expiryTime > 0 ? new Date(data.expiryTime).toLocaleString('zh-CN', { hour12: false }) : data.expiryTime < 0 ? '首次使用后开始计时' : '无固定到期日'}</dd></div><div><dt>服务状态</dt><dd>{state.label}</dd></div></dl>
      </Panel><Panel title="流量使用" className="usage-panel"><p className="usage-caption">本周期已使用</p><p className="usage-amount">{bytes(used)}<span>/ {data.total ? bytes(data.total) : '不限流量'}</span></p><div className="usage-track" role="meter" aria-label="流量使用比例" aria-valuenow={Math.round(percent)} aria-valuemin={0} aria-valuemax={100}><div style={{ width: `${percent}%` }} /></div><p className="usage-left">{data.total ? `剩余 ${bytes(Math.max(0, data.total - used))}` : '当前账户未设置流量上限'}</p><div className="transfer-row"><div><span>上传</span><strong>{bytes(data.upload)}</strong></div><div><span>下载</span><strong>{bytes(data.download)}</strong></div></div></Panel></div>
      <Panel title="订阅地址" action={<span className="section-hint">优先使用本站，原站作为备用</span>}><ErrorNotice error={data.linkError} /><div className="subscription-list">{data.routes.map((route, i) => <div className="subscription-pair" key={route.backup}>
        {data.routes.length > 1 && <h3>订阅 {i + 1}</h3>}
        <div className="subscription-row"><div><label>主用 · 本站中转</label><p className="muted small">由本站服务器获取原站订阅，适合直连原站不稳定时使用。</p>{route.primary ? <code tabIndex={0}>{route.primary}</code> : <p role="status">{route.reason}</p>}</div><Button type="primary" icon={<CopyOutlined />} disabled={!route.primary} onClick={() => copy(route.primary)}>复制主用</Button></div>
        <div className="subscription-row"><div><label>备用 · 原站直连</label><code tabIndex={0}>{route.backup}</code></div><Button icon={<CopyOutlined />} onClick={() => copy(route.backup)}>复制备用</Button></div>
      </div>)}</div>
        <div className="subscription-actions"><p>链接疑似泄露时可以更新地址。重置后，需要在客户端重新导入。</p><Button icon={<ReloadOutlined />} disabled={!data.canRotate} onClick={() => { setResetError(''); setResetOpen(true); }}>重置订阅地址</Button></div>{data.rotateReason && <p className="muted small">{data.rotateReason}</p>}
      </Panel>
    </>}
    <section className="support-strip"><MessageOutlined /><div><h2>需要一点帮助？</h2><p>续期、账户问题或连接异常，都可以联系站长。</p></div>{(data?.supportUrl ?? supportUrl) ? <Button href={data?.supportUrl ?? supportUrl} target="_blank" rel="noopener noreferrer">联系站长 <ExportOutlined /></Button> : <span className="muted">站长暂未设置联系方式</span>}</section>
    <Modal title="重置订阅地址" open={resetOpen} confirmLoading={resetBusy} okText="确认重置" cancelText="取消" closable={!resetBusy} maskClosable={!resetBusy} cancelButtonProps={{ disabled: resetBusy }} okButtonProps={{ disabled: !password }} onCancel={() => { if (!resetBusy) { setResetOpen(false); setPassword(''); } }} onOk={() => void rotate()}>
      <p>主用中转地址和原站备用地址都会更新，你需要重新导入。已经下载的节点凭据不会因此撤销，更新过程也可能影响连接。</p><p>请输入当前账户密码确认操作。</p><Input.Password aria-label="当前账户密码" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} maxLength={72} /><div className="modal-error"><ErrorNotice error={resetError} /></div>
    </Modal>
  </>;
};
