import { useState } from 'react';
import { CheckCircleOutlined, CopyOutlined, LinkOutlined, LockOutlined, ReloadOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { App, Button, Form, Input, Steps } from 'antd';
import { api, errorText } from './api';
import { Brand, ContinueButton, ErrorNotice, Footer, SuccessLine } from './components';
import { passwordRule } from './validation';

type SetupValues = { name: string; suffix: string; username: string; password: string; confirm: string; panelUrl: string; panelToken: string; supportUrl?: string; relayOrigins?: string };
const steps = [ { title: '管理入口', description: '设置站点与管理员' }, { title: '连接面板', description: '验证你的 3x-ui' }, { title: '确认安装', description: '开启独立用户中心' } ];
const fields: (keyof SetupValues)[][] = [['name', 'suffix', 'username', 'password', 'confirm'], ['panelUrl', 'panelToken', 'supportUrl']];

export const Setup = () => {
  const [form] = Form.useForm<SetupValues>();
  const [step, setStep] = useState(0);
  const [busy, setBusy] = useState(false);
  const [checking, setChecking] = useState(false);
  const [checked, setChecked] = useState(false);
  const [error, setError] = useState('');
  const [loginPath, setLoginPath] = useState('');
  const { message } = App.useApp();
  const values = Form.useWatch([], form) as Partial<SetupValues> | undefined;

  const next = async () => {
    try { await form.validateFields(fields[step]); setError(''); setStep(step + 1); } catch { /* 表单字段已展示具体错误。 */ }
  };
  const check = async () => {
    try {
      const v = await form.validateFields(['panelUrl', 'panelToken']); setChecking(true); setError('');
      setChecked(false);
      await api('check', { panelUrl: v.panelUrl, panelToken: v.panelToken });
      setChecked(v.panelUrl === form.getFieldValue('panelUrl') && v.panelToken === form.getFieldValue('panelToken'));
    } catch (e) { if (e instanceof Error) setError(errorText(e)); setChecked(false); }
    finally { setChecking(false); }
  };
  const install = async () => {
    setBusy(true); setError('');
    try {
      const v = form.getFieldsValue(true);
      const result = await api<{ loginPath: string }>('setup', {
        name: v.name, suffix: v.suffix, username: v.username, password: v.password,
        panelUrl: v.panelUrl, panelToken: v.panelToken, supportUrl: v.supportUrl || '', relayOrigins: v.relayOrigins || '',
      });
      form.setFieldsValue({ password: '', confirm: '', panelToken: '' }); setLoginPath(result.loginPath);
    } catch (e) { setError(errorText(e)); } finally { setBusy(false); }
  };
  const generate = () => {
    const bytes = crypto.getRandomValues(new Uint8Array(12));
    form.setFieldValue('suffix', Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join(''));
  };

  return <main className="setup-shell">
    <aside className="setup-aside">
      <Brand light />
      <div className="setup-intro"><h1>从这里，开启<br />你的用户中心。</h1><p>连接现有面板，把订阅与账户服务带到一个清晰、独立的入口。</p></div>
      <Steps orientation="vertical" current={loginPath ? 3 : step} items={steps} className="setup-steps" />
      <div className="aside-note"><SafetyCertificateOutlined /><span>你的站点，你的入口。<br />安装完成后，此安装页面将关闭。</span></div>
    </aside>
    <section className="setup-content">
      <div className="setup-form-wrap">
        {loginPath ? <div className="installation-done">
          <CheckCircleOutlined className="done-icon" /><h1>一切就绪。</h1><p>你的用户中心已创建。请保存下面的专属入口，之后使用管理员账号登录。</p>
          <div className="saved-address"><span>专属登录地址</span><code>{location.origin}{loginPath}</code></div>
          <Button icon={<CopyOutlined />} onClick={() => { void navigator.clipboard.writeText(location.origin + loginPath).then(() => message.success('登录地址已复制')).catch(() => message.error('复制失败，请手动保存上方地址')); }}>复制地址</Button>
          <ContinueButton href={loginPath} block className="finish-button">前往登录</ContinueButton>
        </div> : <>
          <div className="step-caption">步骤 {step + 1} / 3</div>
          <h1>{['建立你的专属入口', '连接已有的 3x-ui 面板', '确认信息，完成安装'][step]}</h1>
          <p className="lead">{['由你设置登录地址和管理员账户，之后从这个入口管理站点。', '使用 API 密钥连接，已有用户和面板配置将继续保留在 3x-ui。', '确认后创建管理员与站点配置，安装入口随即关闭。'][step]}</p>
          <ErrorNotice error={error} />
          <Form form={form} layout="vertical" requiredMark={false} initialValues={{ name: 'Portal' }} onValuesChange={(changed) => { if ('panelUrl' in changed || 'panelToken' in changed) setChecked(false); }}>
            <div hidden={step !== 0}>
              <Form.Item name="name" label="站点名称" rules={[{ required: true, whitespace: true, max: 40, message: '填写 1–40 个字符的站点名称' }]}><Input placeholder="例如：我的用户中心" autoComplete="organization" maxLength={40} /></Form.Item>
              <Form.Item label="登录路径后缀" extra="1–64 位，支持字母、数字、下划线和连字符；sub 为订阅接口保留名称。">
                <div className="suffix-control"><Form.Item name="suffix" noStyle rules={[{ required: true, pattern: /^(?!sub$)[A-Za-z0-9_-]{1,64}$/, message: '请设置 1–64 位有效后缀，不能使用 sub' }]}><Input prefix="/" placeholder="设置专属后缀" autoComplete="off" maxLength={64} /></Form.Item><Button icon={<ReloadOutlined />} onClick={generate}>随机生成</Button></div>
                <div className="path-preview">登录入口：{location.origin}/{values?.suffix || '你的后缀'}/</div>
              </Form.Item>
              <Form.Item name="username" label="管理员账号" rules={[{ required: true, pattern: /^[A-Za-z0-9][A-Za-z0-9_.-]{2,31}$/, message: '3–32 位字母、数字、点、下划线或连字符' }]}><Input autoComplete="username" placeholder="设置管理员账号" maxLength={32} /></Form.Item>
              <div className="form-pair"><Form.Item name="password" label="登录密码" rules={[passwordRule]}><Input.Password autoComplete="new-password" placeholder="输入你的登录密码" /></Form.Item>
                <Form.Item name="confirm" label="确认密码" dependencies={['password']} rules={[{ required: true, message: '请再次输入密码' }, ({ getFieldValue }) => ({ validator(_, value) { return value === getFieldValue('password') ? Promise.resolve() : Promise.reject(new Error('两次输入的密码不一致')); } })]}><Input.Password autoComplete="new-password" placeholder="再次输入密码" /></Form.Item></div>
            </div>
            <div hidden={step !== 1}>
              <Form.Item name="panelUrl" label="3x-ui 面板完整地址" extra="支持 HTTP 或 HTTPS，包含端口和面板路径后缀。HTTP 会明文传输面板密钥，建议通过可信网络连接。" rules={[{ required: true, type: 'url', message: '请输入完整面板地址' }]}><Input prefix={<LinkOutlined />} autoComplete="url" placeholder="https://panel.example.com:8443/面板后缀/" /></Form.Item>
              <Form.Item name="panelToken" label="3x-ui API 密钥" extra="在面板的安全设置中创建 admin 权限 API Token。密钥只在服务端保存。" rules={[{ required: true, min: 16, max: 4096, message: '请输入有效 API Token' }]}><Input.Password prefix={<LockOutlined />} autoComplete="off" placeholder="粘贴 API Token" /></Form.Item>
              <div className="connection-check"><Button onClick={() => void check()} loading={checking}>检查连接</Button>{checked && <SuccessLine>连接成功</SuccessLine>}</div>
              <Form.Item name="supportUrl" label="联系站长（选填）" extra="支持 HTTPS 链接或 mailto:邮箱，之后可在设置中修改。"><Input placeholder="https://t.me/你的用户名" /></Form.Item>
              <Form.Item name="relayOrigins" label="原站订阅来源（选填）" extra="用于本站订阅中转。每行填写协议、域名或 IP 与端口，不含路径；留空仅允许面板同源。独立订阅服务请填写其来源。"><Input.TextArea rows={2} maxLength={2048} placeholder="https://sub.example.com:8443" /></Form.Item>
            </div>
            {step === 2 && <>
              <dl className="review-list"><div><dt>站点名称</dt><dd>{values?.name}</dd></div><div><dt>管理员</dt><dd>{values?.username}</dd></div><div><dt>登录后缀</dt><dd className="mono">/{values?.suffix}/</dd></div><div><dt>面板地址</dt><dd>{values?.panelUrl}</dd></div><div><dt>联系站长</dt><dd>{values?.supportUrl || '稍后设置'}</dd></div></dl>
              <div className="install-note"><LockOutlined /><p>安装时会再次验证密钥。密码与密钥不会显示在此页；请记住你的账号、密码和登录地址。</p></div>
            </>}
          </Form>
          <div className="form-actions">{step > 0 && <Button size="large" disabled={busy} onClick={() => { setError(''); setStep(step - 1); }}>上一步</Button>}<ContinueButton loading={busy} disabled={checking} onClick={() => void (step < 2 ? next() : install())}>{step < 2 ? '继续' : '完成安装'}</ContinueButton></div>
        </>}
        <Footer />
      </div>
    </section>
  </main>;
};
