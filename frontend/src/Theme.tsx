import { useLayoutEffect, useState, type ReactNode } from 'react';
import { App, Button, ConfigProvider, theme } from 'antd';
import { MoonOutlined, SunOutlined } from '@ant-design/icons';
import zhCN from 'antd/locale/zh_CN';

type Mode = 'light' | 'dark';
const readMode = (): Mode => { try { return localStorage.getItem('portal-theme') === 'light' ? 'light' : 'dark'; } catch { return 'dark'; } };
const initial = readMode();
document.documentElement.dataset.theme = initial;
document.documentElement.style.colorScheme = initial;

export const PortalTheme = ({ children }: { children: ReactNode }) => {
  const [mode, setMode] = useState<Mode>(initial); const dark = mode === 'dark';
  useLayoutEffect(() => {
    document.documentElement.dataset.theme = mode; document.documentElement.style.colorScheme = mode;
    try { localStorage.setItem('portal-theme', mode); } catch { /* 隐私模式下仍允许切换当前页面。 */ }
  }, [mode]);
  return <ConfigProvider locale={zhCN} theme={{ algorithm: dark ? theme.darkAlgorithm : theme.defaultAlgorithm, token: {
    colorPrimary: '#1677ff', colorInfo: '#1677ff',
    colorBgBase: dark ? '#1a1b1f' : '#ffffff', colorBgLayout: dark ? '#1a1b1f' : '#f5f5f5',
    colorBgContainer: dark ? '#23252b' : '#ffffff', colorBgElevated: dark ? '#2d2f37' : '#ffffff',
    colorText: dark ? '#f0f0f0' : '#1f1f1f', colorTextSecondary: dark ? '#b0b3be' : '#626262',
    colorTextDescription: dark ? '#b0b3be' : '#626262', colorTextPlaceholder: dark ? '#9a9daa' : '#767676',
    colorBorder: dark ? '#41444f' : '#d9d9d9', borderRadius: 10, controlHeight: 44, fontSize: 14,
    fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
  }, components: dark ? {} : { Button: { colorPrimary: '#0958d9', colorPrimaryHover: '#2468e5', colorPrimaryActive: '#073ea8' } } }}><App>
    <div className="appearance-bar"><span>3x-ui Portal</span><div role="group" aria-label="界面主题"><Button size="small" type={!dark ? 'primary' : 'text'} aria-pressed={!dark} onClick={() => setMode('light')} icon={<SunOutlined aria-hidden="true" />}>明亮</Button><Button size="small" type={dark ? 'primary' : 'text'} aria-pressed={dark} onClick={() => setMode('dark')} icon={<MoonOutlined aria-hidden="true" />}>暗黑</Button></div></div>
    {children}
  </App></ConfigProvider>;
};
