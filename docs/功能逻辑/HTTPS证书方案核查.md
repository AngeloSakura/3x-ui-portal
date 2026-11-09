# HTTPS 证书方案核查

核查日期：2026-11-09。本文记录官方协议、发布记录与源码核查结果，以及实现建议；不代表已经完成公网签发或部署验收。示例 IP 使用文档保留地址，不是真实服务器。

## 1. 可以实现什么

用户所说的“自动申请 SSH”在此业务中应理解为申请网站的 **SSL/TLS 证书**。SSH 登录端口、网站 HTTPS 端口、ACME 验证端口是三个独立概念。

Let's Encrypt 已于 2026-01-15 开放公网 IP 证书，支持 IPv4 和 IPv6。IP 证书必须选择 `shortlived` profile，有效期为 **160 小时**，不能按旧的“90 天证书、到期前 30 天续期”方案处理。[官方公告](https://letsencrypt.org/2026/01/15/6day-and-ip-general-availability/)

| 场景 | 可行方案 | 限制 |
| --- | --- | --- |
| 域名，公网 80 可用 | HTTP-01 | DNS 必须指向能正确响应挑战的入口 |
| 域名，公网 443 可用 | TLS-ALPN-01 | 必须把特殊 ALPN 握手送到挑战处理器，普通 HTTP 反代不够 |
| 域名，只有 NAT 高端口 | DNS-01 + DNS 服务商 API | 不要求网站公网 80/443；需控制域名 DNS |
| 公网 IP，公网 80 可用 | HTTP-01 + `shortlived` | IP 必须属于能控制验证响应的入口 |
| 公网 IP，公网 443 可用 | TLS-ALPN-01 + `shortlived` | 需客户端支持 IP 验证，且握手可达 |
| 公网 IP，只有任意 NAT 高端口 | 当前不能直接通过这些 Let's Encrypt 挑战签发 | DNS-01 不能验证 IP；更换脚本或本地监听端口不能绕过此限制 |

以上挑战边界来自 [Let's Encrypt challenge types](https://letsencrypt.org/docs/challenge-types/)：HTTP-01 固定公网 80，重定向也仅接受 80/443；TLS-ALPN-01 固定公网 443；DNS-01 支持域名但不支持 IP。

**证书可用于非标准网站端口。** 例如拿到覆盖 `portal.example.com` 的证书后，网站可部署为 `https://portal.example.com:8443`；证书校验的是 DNS 名或 IP SAN，端口由网络与服务监听配置决定。lego 的 CSR 源码分别填充 `DNSNames`、`IPAddresses`，不把端口加入 SAN。[CSR 源码](https://github.com/go-acme/lego/blob/v5.6.0/certcrypto/crypto.go)

当前测试 NAT 没有公网 80/443，因此不能把“能用 SSH 登录”或“高端口能通”当作 IP 签发成功条件。可先用本地测试 CA 做 TLS 模拟并明确其信任范围；公网可信验证须改用域名 DNS-01，或由 NAT 提供方提供公网 80/443 验证转发。自签名证书不等于公网受信任证书。

## 2. 推荐固定的 Go 客户端

建议依赖 `github.com/go-acme/lego/v5 v5.6.0`。官方 release 日期为 2026-10-28，其 `go.mod` 要求 Go 1.26.0，与本项目当前技术方向一致。[Release](https://github.com/go-acme/lego/releases/tag/v5.6.0) · [go.mod](https://github.com/go-acme/lego/blob/v5.6.0/go.mod)

仅导入使用的 HTTP-01 与 Cloudflare provider，不导入汇总全部 DNS 服务商的动态工厂。不要混用网上的 v4 示例：v5 的请求方法含 `context.Context`，账户接口、证书资源及续期 API 也有调整。

### 已核实的 v5 API

```go
// 仅展示关键调用；正式代码必须处理每一步错误，并持久化账户密钥与注册信息。
cfg := lego.NewConfig(account)
cfg.CADirURL = lego.DirectoryURLLetsEncrypt
client, err := lego.NewClient(cfg)

registrationResult, err := client.Registration.Register(ctx,
    registration.RegisterOptions{TermsOfServiceAgreed: true})

err = client.Challenge.SetHTTP01Provider(
    http01.NewProviderServer("127.0.0.1", "8088"))

resource, err := client.Certificate.Obtain(ctx, certificate.ObtainRequest{
    Domains: []string{"203.0.113.10"}, // 示例占位符，不能用于真实签发。
    Profile: "shortlived",
    KeyType: certcrypto.EC256,
    Bundle: true,
})
```

`registration.User` 的三个方法是 `GetEmail() string`、`GetRegistration() *acme.ExtendedAccount`、`GetPrivateKey() crypto.Signer`；注册返回 `*acme.ExtendedAccount`。[账户接口](https://github.com/go-acme/lego/blob/v5.6.0/registration/user.go) · [注册 API](https://github.com/go-acme/lego/blob/v5.6.0/registration/registar.go)

上面 `8088` 是**内部挑战监听端口**，必须由公网 **80** 转发到此端口；不代表 CA 会访问公网 8088。也可实现 `Present(ctx, domain, token, keyAuth)` / `CleanUp(ctx, ...)` 的内存挑战 provider，由专用 HTTP 路由响应，避免与主服务抢占监听端口。[HTTP-01 server](https://github.com/go-acme/lego/blob/v5.6.0/challenge/http01/http_challenge_server.go) · [solver manager](https://github.com/go-acme/lego/blob/v5.6.0/challenge/resolver/solver_manager.go)

lego 会把可解析 IP 的 identifier 编码为 `type: ip`，CSR 也会写 IP SAN；不是把 IP 当成普通域名提交。[identifier 源码](https://github.com/go-acme/lego/blob/v5.6.0/acme/api/identifier.go) · [证书请求](https://github.com/go-acme/lego/blob/v5.6.0/certificate/certificates.go)

`lego.NewClient` 构造时就会联网读取 ACME directory，内部使用 background context；必须配置 HTTP 超时，不能在“查看状态”请求或长期持锁区域内构造。[客户端](https://github.com/go-acme/lego/blob/v5.6.0/lego/client.go) · [directory 请求](https://github.com/go-acme/lego/blob/v5.6.0/acme/api/api.go)

### 无标准端口时的域名 DNS-01

Cloudflare 的明确配置 API：

```go
dnsCfg := cloudflare.NewDefaultConfig()
dnsCfg.AuthToken = token
dnsCfg.ZoneToken = token
provider, err := cloudflare.NewDNSProviderConfig(dnsCfg)
err = client.Challenge.SetDNS01Provider(provider)
```

令牌限定目标 Zone，权限为 `Zone:Read` 和 `DNS:Edit`。也可分开使用两个令牌。令牌只在服务端加密保存，不回填到管理页面，不使用全局 API Key。该 provider 会创建 TXT、等待传播、清理对应挑战记录。[Cloudflare provider 源码与权限说明](https://github.com/go-acme/lego/blob/v5.6.0/providers/dns/cloudflare/cloudflare.go)

## 3. 续期与后台管理建议

以下是本项目设计建议，应由实现与测试分别确认，不把建议写成已上线功能。

1. 持久化账户密钥、注册信息、签发配置、证书元信息、最近任务结果。私钥使用受限权限和加密存储；管理 API 仅返回公开状态及到期时间。
2. 后台明确区分“申请参数”“正在使用的证书”。管理员修改域名/IP 后，先申请并校验 SAN、证书链、有效期及私钥匹配，再原子替换；失败保留旧证书和旧入口。
3. 签发、重新申请、自动续期共用串行任务锁；配置带版本号，过时任务不能覆盖新的配置。写接口要求管理员身份、CSRF、密码二次确认、限速及操作审计。
4. 通过 `GetRenewalInfo(ctx, leafCertificate)` 获取 ARI，调用 `ShouldRenewAt` 选择续期时间；通过 `Renew(ctx, resource, &certificate.RenewOptions{Profile: profile, Bundle: true, UseARICertID: true})` 更新。ARI 不可用时，基于实际 `NotBefore/NotAfter` 的寿命比例提前续期；160 小时证书不能等待 30 天阈值。
5. 定时任务定期检查，重启后继续；失败采用有上限的退避并保留旧证书。显示最近错误、下次尝试、剩余时间。临近到期应明显告警，过期不能静默降级为 HTTP。
6. Go HTTPS listener 使用 `tls.Config.GetCertificate` 读取已校验的当前证书指针；完整落盘后再替换指针。非标准 HTTPS 端口由监听与公网映射分别配置，HTTP challenge 路由不能被登录后缀或管理员鉴权拦截。
7. 改变公开 origin 与证书名称属于协同变更：更新生成的订阅链接、Host/Origin 白名单与会话策略，并提示新的管理入口。不能只替换证书而遗留错误的本站链接。

上述续期 API 及 ARI 行为见 [lego v5.6.0 renewal.go](https://github.com/go-acme/lego/blob/v5.6.0/certificate/renewal.go)。协议原理见 [RFC 9773](https://www.rfc-editor.org/rfc/rfc9773.html)。TLS 热加载 API 见 [Go tls.Config](https://pkg.go.dev/crypto/tls#Config)。

## 4. 其他客户端对比与验证边界

| 客户端 | 本次一手来源能确认的事实 | 本项目选择 |
| --- | --- | --- |
| lego v5.6.0 | IP identifier、IP SAN、profile、HTTP-01、DNS provider、ARI 均有固定版本源码 | 适合直接嵌入 Go 后台，推荐 |
| Certbot | 官方 2026-03-11 文章说明 `--ip-address` 来自 5.3，webroot IP 建议 5.4+；`--preferred-profile shortlived` | 可作为外部运维替代，但要额外处理 deploy hook 与进程权限 |
| acme.sh | 主分支已有 profile 与基于实际寿命/ARI 的续期代码；本次未完成固定 release 全链路核查 | 不作为本次集成依赖，不提供未经固定版本验证的安装管道 |
| Caddy | 官方 automatic HTTPS 页仍将 IP 自动处理描述为本地 CA；同时明确内部端口可变而公网 HTTP/TLS 挑战仍需 80/443 | 不把默认 `IP { ... }` 配置当作公网 IP 证书保证 |

来源：[Certbot 官方说明](https://letsencrypt.org/2026/03/11/shorter-certs-certbot/) · [acme.sh 官方源码](https://github.com/acmesh-official/acme.sh/blob/master/acme.sh) · [Caddy automatic HTTPS](https://caddyserver.com/docs/automatic-https)

Certbot 官方给出的测试命令形式为：

```sh
certbot certonly --staging --preferred-profile shortlived \
  --webroot --webroot-path /var/www/acme \
  --ip-address <PUBLIC_IP>
```

这要求公网 80 的 challenge 可达；`--staging` 产出证书不受浏览器信任。去掉 `--staging` 才是正式 CA，仍需满足验证与限额。该命令仅为官方用法说明，本文核查没有执行签发。

本次只读取官方网页、GitHub API 与固定 tag 源码，没有使用真实服务器凭据、没有运行客户端、没有申请证书、没有修改 DNS。公网签发结果必须由后续真实验证证据单独记录。
