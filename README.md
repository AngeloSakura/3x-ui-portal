# 3x-ui Portal

独立部署的 3x-ui 用户自助端。Go + Gin 后端、React + TypeScript + Ant Design 前端，独立 SQLite 数据库；仅通过面板 HTTP API 对接，不修改 3x-ui，不访问其数据库。

作者：[AngeloSakura](https://github.com/AngeloSakura) · [问题与建议](https://github.com/AngeloSakura/3x-ui-portal/issues) · [发行版](https://github.com/AngeloSakura/3x-ui-portal/releases)

## 功能

- 安装时自设登录后缀、管理员账号密码和 3x-ui API Token，无默认密码。后缀支持 1–64 位；密码按 3x-ui 非空规则，不强制复杂度，最长 72 个 UTF-8 字节。
- 面板可使用 HTTP 或 HTTPS；用户端公开访问要求 HTTPS。
- 邀请码注册：站长选择已有客户端生成专属邀请码，自动沿用客户端名称并绑定账号，无需另填本站账号。用户兑换后设置密码，直接进入首页；以后用账号密码登录。
- 到期倒计时、流量、账号状态，每 15 秒同步；联系站长、修改密码。
- 主订阅由本站服务器代取，备用订阅展示原站地址；中转来源需站长明确配置。
- 自助重置订阅默认关闭，限制共享订阅、异常状态和远端入站，失败不盲目重试。
- 内置 HTTPS：IP / 域名 HTTP-01、Cloudflare 域名 DNS-01、证书导入；后台配置与重申请，自动续期并热更新。
- 与 3x-ui 一致的明亮/暗黑主题，可切换并记住选择；全站保留可点击作者主页。

## 安装

支持带 systemd 或 OpenRC（Alpine）的 Linux amd64 / arm64。运行时无需 Go、Node、GCC。全新安装：

```bash
curl -fL https://raw.githubusercontent.com/AngeloSakura/3x-ui-portal/main/scripts/install-linux.sh -o install-portal.sh
# 先阅读脚本，再执行；网址改成你自己的公开地址
sudo bash install-portal.sh https://portal.example.com:8443 cloudflare
```

模式可选 `http`、`cloudflare`、`import`。安装器下载 GitHub Release，校验 SHA-256，初始化独立数据库并安装 systemd / OpenRC 服务。已有路径会拒绝覆盖。按照提示读取一次性安装凭证，再到浏览器设置账号、后缀及面板密钥。

**NAT 与证书：**网站可以使用任意映射端口，但 HTTP-01 必须从公网 80 到达验证监听端口。只有 NAT 高端口时，不能直接申请公网可信 IP 证书。域名 DNS-01 无需公网 80/443。`test-self-signed` 仅用于模拟 HTTPS，不代表可信证书签发成功，客户端不会自动信任。

## 文档

- [部署与安装](docs/部署与安装.md)：证书申请、端口、编译、升级与恢复。
- [操作手册](docs/操作手册.md)：管理员及用户流程、故障处理。
- [HTTPS 实现](docs/功能逻辑/HTTPS管理.md)与[官方方案核查](docs/功能逻辑/HTTPS证书方案核查.md)。
- [安装与账户安全](docs/功能逻辑/安装与账户安全.md)、[订阅中转与首次激活](docs/功能逻辑/订阅中转与首次激活.md)。
- [API 兼容性与重置边界](docs/功能逻辑/3x-ui对接与用户自助.md)、[SQL 升级](docs/sql/README.md)。
- [验证记录](docs/验证记录.md)、[参与维护](CONTRIBUTING.md)。
- [开发约束](AGENTS.md)及[项目必读技能](.agents/skills/portal-development/SKILL.md)。

## 当前验证状态

[v0.1.2](https://github.com/AngeloSakura/3x-ui-portal/releases/tag/v0.1.2) 已通过 GitHub Actions 构建，提供 Linux amd64 / arm64 包和 SHA-256 校验文件。已在低内存 Alpine / OpenRC NAT 环境从 GitHub 安装并升级至 v0.1.2，验证公网 HTTPS 页面、手机布局、主题保持、服务重启及证书状态恢复。本地模拟完成安装、邀请、强制设密码、登录、订阅中转和重置保护测试。

NAT 使用自签测试证书，尚未验证真实 CA 签发与目标 3x-ui 实际联调；域名测试待后续进行。详见[验证记录](docs/验证记录.md)。

## 兼容性与许可

对接契约基于 3x-ui 3.11.0 新客户端 API。当前使用 admin API Token，未实现面板用户名密码登录、支付、开放注册、多面板及管理员密码找回界面。模拟测试不替代真实面板与实际订阅客户端验收，正式使用前请用测试用户核实目标版本，尤其是订阅重置。

采用 [PolyForm Noncommercial 1.0.0](LICENSE)。允许许可范围内的修改、分发与非商业使用，商业使用需另外授权。这是源码公开的非商业许可，**不属于 OSI 定义的开源许可**。依赖仍适用各自许可。欢迎提交 Issue 或 Pull Request 维护项目。
