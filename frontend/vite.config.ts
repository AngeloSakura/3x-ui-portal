import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// 相对资源路径同时支持一次性安装入口和用户自定义登录后缀。
export default defineConfig({ plugins: [react()], base: './', build: { sourcemap: false } });
