import React from 'react';
import ReactDOM from 'react-dom/client';
import { PortalTheme } from './Theme';
import { PortalApp } from './App';
import './styles.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode><PortalTheme><PortalApp /><footer className="author-footer">3x-ui Portal · <a href="https://github.com/AngeloSakura" target="_blank" rel="noreferrer">AngeloSakura</a><span>源码公开 · 非商业使用</span></footer></PortalTheme></React.StrictMode>,
);
