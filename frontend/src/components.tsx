import { ArrowRightOutlined, CheckOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { Alert, Button } from 'antd';
import type { ComponentProps, ReactNode } from 'react';

export const Brand = ({ name = 'Portal', light = false }: { name?: string; light?: boolean }) => (
  <div className={`brand ${light ? 'brand-light' : ''}`}>
    <svg viewBox="0 0 32 32" width="32" height="32" aria-hidden="true"><rect width="32" height="32" rx="10" fill="currentColor" /><path d="M10 22V10h7a5 5 0 0 1 0 10h-3M14 14h3a1 1 0 0 1 0 2h-3" fill="none" stroke="white" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" /></svg>
    <span>{name}</span>
  </div>
);

export const ErrorNotice = ({ error }: { error: string }) => error ? <Alert type="error" showIcon title={error} className="notice" /> : null;
export const Footer = () => <footer className="quiet-footer"><SafetyCertificateOutlined /> 独立账户 · 专属访问入口</footer>;
export const Panel = ({ title, action, children, className = '' }: { title: string; action?: ReactNode; children: ReactNode; className?: string }) => (
  <section className={`panel ${className}`}><div className="section-heading"><h2>{title}</h2>{action}</div>{children}</section>
);
export const ContinueButton = ({ children, ...props }: ComponentProps<typeof Button>) => <Button type="primary" size="large" {...props}>{children}<ArrowRightOutlined aria-hidden="true" /></Button>;
export const SuccessLine = ({ children }: { children: ReactNode }) => <p className="success-line" role="status"><CheckOutlined />{children}</p>;
