import { useEffect, useRef, useState } from 'react';
import { App, Button, Drawer, Empty, Form, Input, Popconfirm, Select, Skeleton, Switch, Table, Tag } from 'antd';
import { PlusOutlined, SearchOutlined } from '@ant-design/icons';
import { api, errorText, type Invitation, type Settings, type User } from './api';
import { ErrorNotice, Panel } from './components';

type Choice = { id: number; email: string; enable: boolean };
type CreateValues = { username: string; clientId: number };

export const AdminUsers = () => {
  const [users, setUsers] = useState<User[]>([]), [total, setTotal] = useState(0), [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true), [error, setError] = useState(''), [revision, setRevision] = useState(0);
  const [open, setOpen] = useState(false), [saving, setSaving] = useState(false), [createError, setCreateError] = useState('');
  const [search, setSearch] = useState(''), [searching, setSearching] = useState(false), [choices, setChoices] = useState<Choice[]>([]);
  const [changing, setChanging] = useState<number>();
  const [invitation, setInvitation] = useState<Invitation>();
  const [inviting, setInviting] = useState<number>();
  const searchRequest = useRef<AbortController | undefined>(undefined);
  const [form] = Form.useForm<CreateValues>(); const { message } = App.useApp();
  useEffect(() => {
    const ctrl = new AbortController(); setLoading(true);
    void api<{ items: User[]; total: number }>(`admin/users?page=${page}`, undefined, ctrl.signal).then((v) => { setUsers(v.items); setTotal(v.total); setError(''); }).catch((e: unknown) => { if (!ctrl.signal.aborted) setError(errorText(e)); }).finally(() => { if (!ctrl.signal.aborted) setLoading(false); });
    return () => ctrl.abort();
  }, [page, revision]);
  useEffect(() => () => searchRequest.current?.abort(), []);
  const cancelSearch = () => { searchRequest.current?.abort(); setSearching(false); };
  const find = async () => {
    searchRequest.current?.abort();
    const ctrl = new AbortController(); searchRequest.current = ctrl;
    setSearching(true); setCreateError(''); setChoices([]); form.resetFields(['clientId']);
    try { const result = await api<{ items: Choice[] }>(`admin/clients?search=${encodeURIComponent(search)}`, undefined, ctrl.signal); if (!ctrl.signal.aborted) setChoices(result.items); }
    catch (e) { if (!ctrl.signal.aborted) setCreateError(errorText(e)); } finally { if (!ctrl.signal.aborted) setSearching(false); }
  };
  const create = async (v: CreateValues) => {
    const chosen = choices.find((c) => c.id === v.clientId); if (!chosen) { setCreateError('请先选择面板客户端'); return; }
    setSaving(true); setCreateError('');
    try { const result = await api<Invitation>('admin/users', { username: v.username, email: chosen.email, clientId: chosen.id }); setInvitation(result); message.success('用户已创建，请将激活链接交给本人'); cancelSearch(); form.resetFields(); setOpen(false); setRevision((n) => n + 1); setPage(1); }
    catch (e) { setCreateError(errorText(e)); } finally { setSaving(false); }
  };
  const changeState = async (u: User) => {
    setChanging(u.id);
    try { await api('admin/user-state', { id: u.id, disabled: !u.disabled }); message.success('门户账号状态已更新'); setRevision((n) => n + 1); }
    catch (e) { message.error(errorText(e)); } finally { setChanging(undefined); }
  };
  const issue = async (id: number) => {
    setInviting(id);
    try { setInvitation(await api<Invitation>('admin/invite', { id })); message.success('新的激活链接已生成，旧链接已失效'); }
    catch (e) { message.error(errorText(e)); } finally { setInviting(undefined); }
  };
  return <><ErrorNotice error={error} />{invitation && <Panel title={`将激活链接交给 ${invitation.username} 本人`} action={<Button type="text" onClick={() => setInvitation(undefined)}>收起</Button>}><p className="muted">有效至 {new Date(invitation.expiresAt * 1000).toLocaleString('zh-CN')}，仅能使用一次。用户首次无需密码，进入后自行设置本站密码。</p><div className="subscription-row"><code className="invitation-address" tabIndex={0}>{invitation.activationUrl}</code><Button onClick={() => { void navigator.clipboard.writeText(invitation.activationUrl).then(() => message.success('已复制激活链接')).catch(() => message.error('复制失败，请手动选中链接复制')); }}>复制激活链接</Button></div></Panel>}<Panel title={`用户账户${total ? ` · ${total}` : ''}`} action={<Button type="primary" icon={<PlusOutlined />} onClick={() => { cancelSearch(); form.resetFields(); setCreateError(''); setChoices([]); setSearch(''); setOpen(true); }}>添加用户</Button>}>
    <p className="muted">绑定已有客户端后发送一次性邀请，密码由用户首次进入时设置。停用会同时关闭本站登录与订阅中转，不修改 3x-ui。</p>
    <Table<User> rowKey="id" loading={loading} dataSource={users} scroll={{ x: 650 }} pagination={{ current: page, pageSize: 20, total, showSizeChanger: false, onChange: setPage }} locale={{ emptyText: <Empty description="还没有用户。点击“添加用户”，为面板客户端建立登录账户。" /> }} columns={[
      { title: '登录账号', dataIndex: 'username', render: (text: string) => <strong>{text}</strong> },
      { title: '面板客户端', dataIndex: 'clientEmail' },
      { title: '状态', render: (_, u) => <Tag color={u.disabled ? 'default' : u.mustSetPassword ? 'orange' : 'green'}>{u.disabled ? '已停用' : u.mustSetPassword ? '待设置密码' : '已激活'}</Tag> },
      { title: '创建时间', dataIndex: 'createdAt', render: (n: number) => new Date(n * 1000).toLocaleDateString('zh-CN') },
      { title: '启用账户', render: (_, u) => <Switch aria-label={`启用 ${u.username} 的账户`} checked={!u.disabled} loading={changing === u.id} disabled={changing !== undefined} onChange={() => void changeState(u)} /> },
      { title: '首次激活', render: (_, u) => u.mustSetPassword ? <Popconfirm title="重新生成激活链接？" description="旧链接及未完成的激活会话将失效。" okText="重新生成" cancelText="取消" onConfirm={() => issue(u.id)}><Button size="small" disabled={u.disabled || inviting !== undefined} loading={inviting === u.id}>重发邀请</Button></Popconfirm> : '已完成' },
    ]} />
  </Panel><Drawer title="添加用户" open={open} onClose={() => { if (!saving) { cancelSearch(); setOpen(false); form.resetFields(); } }} width={480} closable={!saving} maskClosable={!saving}>
    <p className="muted">选择已有客户端并设置登录账号，创建后会生成激活链接。无需代替用户设置密码。</p><ErrorNotice error={createError} />
    <label className="field-label" htmlFor="client-search">查找面板客户端</label><div className="client-search"><Input id="client-search" placeholder="输入客户端名称" value={search} disabled={saving} onChange={(e) => { cancelSearch(); setSearch(e.target.value); setChoices([]); form.resetFields(['clientId']); }} onPressEnter={() => void find()} maxLength={128} /><Button icon={<SearchOutlined />} onClick={() => void find()} loading={searching} disabled={saving}>查找</Button></div>
    <Form form={form} layout="vertical" requiredMark={false} onFinish={(v) => void create(v)}>
      <Form.Item label="绑定客户端" name="clientId" rules={[{ required: true, message: '请选择客户端' }]}><Select placeholder="先查找，再选择客户端" options={choices.map((c) => ({ value: c.id, label: `${c.email}${c.enable ? '' : '（已停用）'}` }))} notFoundContent="没有结果，请修改搜索条件" /></Form.Item>
      <Form.Item label="用户登录账号" name="username" rules={[{ required: true, pattern: /^[A-Za-z0-9][A-Za-z0-9_.-]{2,31}$/, message: '3–32 位字母、数字、点、下划线或连字符' }]}><Input autoComplete="off" maxLength={32} placeholder="例如：lin" /></Form.Item>
      <Button type="primary" htmlType="submit" loading={saving} block>创建用户</Button>
    </Form>
  </Drawer></>;
};

export const SiteSettings = ({ onSaved }: { onSaved: (name: string) => void }) => {
  const [settings, setSettings] = useState<Settings>(), [error, setError] = useState(''), [saving, setSaving] = useState(false);
  const [form] = Form.useForm<Settings & { panelToken?: string }>(); const { message } = App.useApp();
  useEffect(() => { const ctrl = new AbortController(); void api<Settings>('admin/settings', undefined, ctrl.signal).then((v) => { setSettings(v); form.setFieldsValue(v); }).catch((e: unknown) => { if (!ctrl.signal.aborted) setError(errorText(e)); }); return () => ctrl.abort(); }, [form]);
  const save = async (v: Settings & { panelToken?: string }) => {
    setSaving(true); setError('');
    try { await api('admin/settings', { name: v.name, supportUrl: v.supportUrl || '', panelToken: v.panelToken || '', allowRotation: !!v.allowRotation, relayOrigins: v.relayOrigins || '' }); form.setFieldValue('panelToken', ''); onSaved(v.name); message.success('设置已保存'); }
    catch (e) { setError(errorText(e)); } finally { setSaving(false); }
  };
  return <Panel title="站点与面板连接" className="narrow-panel"><ErrorNotice error={error} />{!settings ? <Skeleton active /> : <Form form={form} layout="vertical" requiredMark={false} onFinish={(v) => void save(v)}>
    <Form.Item label="站点名称" name="name" rules={[{ required: true, whitespace: true, max: 40, message: '请输入站点名称' }]}><Input maxLength={40} /></Form.Item>
    <Form.Item label="3x-ui 面板地址" extra="当前用户绑定此面板。为避免账号错绑，此处不直接切换面板。"><Input value={settings.panelUrl} readOnly /></Form.Item>
    <Form.Item label="替换 API 密钥" name="panelToken" extra="留空保留现有密钥；新密钥验证通过后才保存。"><Input.Password placeholder="仅在需要更换时填写" autoComplete="off" /></Form.Item>
    <Form.Item label="联系站长" name="supportUrl" extra="HTTPS 网址或 mailto:邮箱链接。"><Input placeholder="https://t.me/你的用户名" /></Form.Item>
    <Form.Item label="允许中转的原站订阅来源" name="relayOrigins" extra="每行填写协议、域名或 IP、端口，不含路径。留空仅允许面板同源；独立订阅端口或域名需在这里填写。"><Input.TextArea rows={3} maxLength={2048} placeholder="https://sub.example.com:8443" /></Form.Item>
    <div className="settings-divider" /><Form.Item label="开放自助重置订阅" name="allowRotation" valuePropName="checked" extra="默认关闭。启用前请在测试账户核实面板兼容性与连接影响；共享、异常或远端入站仍会被拒绝。"><Switch /></Form.Item>
    <Button type="primary" htmlType="submit" loading={saving}>保存设置</Button>
  </Form>}</Panel>;
};
