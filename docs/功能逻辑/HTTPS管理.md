# HTTPS 管理

## 流程与权限

站点公开地址固定来自 PORTAL_ORIGIN，托管模式由 Go TLS 服务直接监听；代理模式仅监听回环。初次证书由安装脚本/CLI 准备，后台需要管理员会话和 CSRF。证书主机不接受普通用户输入。更换域名需由服务器操作者同步迁移公开地址，避免签发和登录源不一致。

`GET api/admin/certificate` 返回脱敏状态；`POST api/admin/certificate` 保存邮箱、验证方式、测试 CA 选择、协议同意和 DNS Token；`POST api/admin/certificate/issue` 启动异步任务。普通用户不能访问管理接口。DNS Token 从不回显，空值保留，HTTP 模式清除 DNS Token。

## 存储与并发

数据目录 `tls-state.enc` 保存整体 AES-GCM 密文：配置、ACME 账户私钥、证书元信息、PEM、申请状态。使用现有独立主密钥，不改变数据库模式。lego v5 Resource 的 PEM 字段不参与 JSON，因此由应用显式持久化。临时文件 0600，写入、同步后原子替换；证书配对、SAN 和有效期校验通过后才替换内存证书。

单进程单任务锁；申请过程中禁止改配置。最近申请时间在联网前持久化，两次申请至少间隔一小时，重启不能绕过。CLI 维护须停服务，避免跨进程覆盖。失败保留上次可用证书，后台不返回包含秘密的原始 ACME/DNS 错误。

## 续期与边界

每 15 分钟检查，到实际有效期 2/3 时续期；使用 lego Renew 的 ARI replacement 标识，但当前调度采用本地寿命比例，不查询 ARI suggestedWindow。申请最多 10 分钟，单 HTTP 请求最多 30 秒；DNS 传播遵循 provider 的有限超时。服务停止期间不能续期，恢复后补检查。

IP 使用 shortlived profile，HTTP-01 必须公网 80；Cloudflare DNS-01 仅域名。自动签发支持 Let's Encrypt 正式与测试环境，不接受任意 CA 地址。导入证书可用于已有 CA 或隔离测试；未配置 ACME 时不自动续期，UI 不声称导入证书必定可信。

本地 Go 测试使用注入签发器验证成功/失败更新、密文重启恢复、真实 TLS 握手与 SAN 拒绝；浏览器模拟使用自签证书。它们不等于真实 ACME CA 订单已通过。真实 NAT IP 签发受公网验证入口限制，详见验证记录。
