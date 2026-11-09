import { useEffect, useState } from 'react';
import { App, Button, Checkbox, Form, Input, Popconfirm, Select, Skeleton, Space, Tag } from 'antd';
import { api, errorText } from './api';
import { ErrorNotice, Panel } from './components';

type TLSSettings = { email: string; method: string; staging: boolean; dnsToken?: string; agreeTerms: boolean };
type TLSStatus = { host: string; settings: TLSSettings; hasDnsToken: boolean; busy: boolean; source: string; notAfter: string; renewAt: string; lastAttempt: string; lastError: string };
const date = (v?: string) => !v || v.startsWith('0001') ? '尚无记录' : new Date(v).toLocaleString('zh-CN');

export const Certificates = () => {
  const [status, setStatus] = useState<TLSStatus>();
  const [managed, setManaged] = useState<boolean>();
  const [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const [form] = Form.useForm<TLSSettings>(); const { message } = App.useApp();
  const method = Form.useWatch('method', form);
  useEffect(() => {
    const controller = new AbortController(); let initial = true;
    const refresh = async () => {
      try {
        const result = await api<{ managed: boolean; status?: TLSStatus }>('admin/certificate', undefined, controller.signal);
        if (controller.signal.aborted) return;
        setManaged(result.managed); setStatus(result.status);
        if (initial && result.status) { form.setFieldsValue({ ...result.status.settings, method: result.status.settings.method || 'http' }); initial = false; }
      } catch (e) { if (!controller.signal.aborted) setError(errorText(e)); }
    };
    void refresh(); const timer = window.setInterval(() => void refresh(), 5000);
    return () => { controller.abort(); clearInterval(timer); };
  }, [form]);
  const save = async (values: TLSSettings) => {
    setBusy(true); setError('');
    try { await api('admin/certificate', { ...values, staging: !!values.staging, agreeTerms: !!values.agreeTerms }); form.setFieldValue('dnsToken', ''); message.success('配置已保存，自动续期会使用新配置'); }
    catch (e) { setError(errorText(e)); } finally { setBusy(false); }
  };
  const issue = async () => {
    setBusy(true); setError('');
    try { await api('admin/certificate/issue', {}); setStatus((s) => s ? { ...s, busy: true } : s); message.success('申请已开始，结果会自动更新'); }
    catch (e) { setError(errorText(e)); } finally { setBusy(false); }
  };
  return <Panel title="HTTPS 证书" className="settings-panel"><ErrorNotice error={error} />
    {managed === undefined && !error && <Skeleton active />}
    {managed === false && <p className="muted">当前 HTTPS 由外部反向代理管理。请在反向代理中续期，或按部署手册切换为本站托管模式。</p>}
    {managed && status && <>
      <div className="certificate-status"><div><strong>{status.host}</strong><p className="muted">网站可使用非标准端口，证书绑定域名或 IP。</p></div><Tag color={status.source === 'letsencrypt' ? 'green' : 'orange'}>{status.busy ? '正在申请' : status.source === 'letsencrypt' ? '正式证书' : status.source === 'staging' ? '测试 CA · 不受浏览器信任' : status.source === 'imported' ? '导入证书 · 信任由签发方决定' : '尚未签发'}</Tag></div>
      <dl className="certificate-dates"><div><dt>到期时间</dt><dd>{date(status.notAfter)}</dd></div><div><dt>自动续期计划</dt><dd>{date(status.renewAt)}</dd></div><div><dt>最近申请</dt><dd>{date(status.lastAttempt)}</dd></div></dl>
      <ErrorNotice error={status.lastError} />
      <Form form={form} layout="vertical" requiredMark={false} onFinish={(v) => void save(v)} disabled={busy || status.busy}>
        <Form.Item name="email" label="证书联系邮箱" rules={[{ required: true, type: 'email', message: '请输入有效邮箱' }]}><Input autoComplete="email" placeholder="admin@example.com" /></Form.Item>
        <Form.Item name="method" label="验证方式" extra={method === 'cloudflare' ? '无需开放公网 80/443，域名 DNS 必须由 Cloudflare 托管。' : '必须将公网 80 转发到安装时配置的验证端口。只有 NAT 高端口时无法完成 IP 验证。'}><Select options={[{ value: 'http', label: 'HTTP-01 · IP / 域名' }, { value: 'cloudflare', label: 'Cloudflare DNS-01 · 仅域名' }]} /></Form.Item>
        {method === 'cloudflare' && <Form.Item name="dnsToken" label="Cloudflare API Token" extra={status.hasDnsToken ? '已保存；留空保留。权限限目标域名 Zone:Read 与 DNS:Edit。' : '权限限目标域名 Zone:Read 与 DNS:Edit；凭据加密保存在本站。'}><Input.Password autoComplete="off" placeholder="填写或替换 API Token" /></Form.Item>}
        <Form.Item name="staging" valuePropName="checked"><Checkbox>使用测试 CA（仅调试，浏览器不会信任）</Checkbox></Form.Item>
        <Form.Item name="agreeTerms" valuePropName="checked" rules={[{ validator: (_, v: boolean) => v ? Promise.resolve() : Promise.reject(new Error('请先阅读并同意协议')) }]}><Checkbox>我同意 <a href="https://letsencrypt.org/repository/" target="_blank" rel="noreferrer">Let's Encrypt 订阅者协议</a></Checkbox></Form.Item>
        <Space wrap><Button type="primary" htmlType="submit" loading={busy}>保存配置</Button><Popconfirm title="使用已保存的配置申请证书？" description="两次申请至少间隔一小时。失败保留旧证书。" onConfirm={() => void issue()} okText="开始申请" cancelText="取消"><Button disabled={!status.settings.method || busy || status.busy}>重新申请</Button></Popconfirm></Space>
      </Form><p className="muted certificate-help">服务每 15 分钟检查一次，在证书有效期约剩三分之一时自动续期。更换站点域名或 IP 需同步修改公开地址，详见部署手册。</p>
    </>}
  </Panel>;
};
