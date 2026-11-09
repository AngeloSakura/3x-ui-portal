// bcrypt 限制按 UTF-8 字节计算，与后端校验保持一致。
export const passwordRule = {
  validator(_: unknown, value: unknown) {
    if (typeof value !== 'string' || !value) return Promise.reject(new Error('请输入密码'));
    const length = new TextEncoder().encode(value).length;
    if (value.trim() !== value || length < 12 || length > 72) {
      return Promise.reject(new Error('密码须为 12–72 字节，首尾不能有空格；一个中文通常占 3 字节'));
    }
    return Promise.resolve();
  },
};
