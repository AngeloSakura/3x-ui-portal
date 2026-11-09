#!/usr/bin/env bash
# 从 GitHub Release 安装；不会修改 3x-ui、SSH 或已有门户数据。
set -euo pipefail
[[ ${EUID} -eq 0 ]] || { echo '请使用 sudo bash 执行。' >&2; exit 1; }
for portal_tool in curl tar sha256sum openssl su useradd install; do
  command -v "$portal_tool" >/dev/null || { echo "缺少工具：$portal_tool" >&2; exit 1; }
done
if command -v systemctl >/dev/null && [[ -d /run/systemd/system ]]; then portal_init=systemd
elif command -v rc-service >/dev/null; then portal_init=openrc
else echo '需要 systemd 或 OpenRC 服务管理器。' >&2; exit 1; fi
portal_origin=${1:-}
portal_mode=${2:-}
if [[ -z "$portal_origin" ]]; then read -rp '本站公开 HTTPS 地址（含网站端口，不含路径）：' portal_origin; fi
if [[ "$portal_origin" =~ ^https://(\[[0-9a-fA-F:]+\]|[A-Za-z0-9.-]+)(:([0-9]{1,5}))?$ ]]; then
  portal_host=${BASH_REMATCH[1]}; portal_host=${portal_host#[}; portal_host=${portal_host%]}
  portal_port=${BASH_REMATCH[3]:-443}
  portal_port=$((10#$portal_port))
  (( portal_port >= 1 && portal_port <= 65535 )) || { echo '端口无效'; exit 1; }
else echo '需要无路径的 HTTPS 地址'; exit 1; fi
if [[ -z "$portal_mode" ]]; then
  echo '证书模式：http（公网 80 验证）、cloudflare（域名 DNS 验证）、import（已有证书）。'
  read -rp '请选择模式：' portal_mode
fi
case "$portal_mode" in http|cloudflare|import|test-self-signed) ;; *) echo '证书模式无效。' >&2; exit 1;; esac
portal_install=/opt/3x-ui-portal
portal_data=/var/lib/3x-ui-portal
portal_config=/etc/3x-ui-portal
for portal_target in "$portal_install" "$portal_data" "$portal_config" /etc/systemd/system/3x-ui-portal.service /etc/init.d/3x-ui-portal; do
  [[ ! -e "$portal_target" ]] || { echo "已有路径 $portal_target；请按升级手册操作，不覆盖。" >&2; exit 1; }
done
case "$(uname -m)" in x86_64) portal_arch=amd64;; aarch64|arm64) portal_arch=arm64;; *) echo '仅支持 Linux amd64/arm64。' >&2; exit 1;; esac
portal_tmp=$(mktemp -d /tmp/portal-install.XXXXXXXX)
trap 'rm -rf -- "$portal_tmp"' EXIT
portal_archive="3x-ui-portal-linux-$portal_arch.tar.gz"
portal_version=${PORTAL_VERSION:-latest}
if [[ "$portal_version" == latest ]]; then portal_release=https://github.com/AngeloSakura/3x-ui-portal/releases/latest/download
else
  [[ "$portal_version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-a-zA-Z0-9.]*)?$ ]] || { echo '版本格式无效'; exit 1; }
  portal_release="https://github.com/AngeloSakura/3x-ui-portal/releases/download/$portal_version"
fi
curl --fail --location --proto '=https' --tlsv1.2 "$portal_release/$portal_archive" -o "$portal_tmp/$portal_archive"
curl --fail --location --proto '=https' --tlsv1.2 "$portal_release/SHA256SUMS" -o "$portal_tmp/SHA256SUMS"
(cd "$portal_tmp"; awk -v file="$portal_archive" '$2 == file {print}' SHA256SUMS > CHECKSUM; [[ -s CHECKSUM ]]; sha256sum -c CHECKSUM)
tar -xzf "$portal_tmp/$portal_archive" -C "$portal_tmp"
id portal-svc >/dev/null 2>&1 || useradd --system --user-group --home-dir "$portal_data" --shell /sbin/nologin portal-svc
install -d -m 0755 "$portal_install" "$portal_install/web"
install -m 0755 "$portal_tmp/portal" "$portal_install/portal"
if [[ "$portal_init" == openrc ]]; then
  command -v setcap >/dev/null || { echo 'OpenRC 模式需要 setcap（Alpine: apk add libcap-setcap）。'; exit 1; }
  setcap cap_net_bind_service=+ep "$portal_install/portal"
fi
cp -R "$portal_tmp/web/." "$portal_install/web/"
install -d -m 0700 -o portal-svc -g portal-svc "$portal_data"
install -d -m 0750 -o portal-svc -g portal-svc "$portal_config" "$portal_config/secrets"
portal_acme_listen=${PORTAL_ACME_LISTEN:-0.0.0.0:80}
[[ "$portal_acme_listen" =~ ^(\[[0-9a-fA-F:]+\]|[A-Za-z0-9.-]*):([0-9]{1,5})$ ]] || { echo '验证监听地址格式无效'; exit 1; }
portal_acme_port=$((10#${BASH_REMATCH[2]}))
(( portal_acme_port >= 1 && portal_acme_port <= 65535 )) || { echo '验证监听端口无效'; exit 1; }
cat > "$portal_config/portal.env" <<EOF
PORTAL_LISTEN=0.0.0.0:$portal_port
PORTAL_ORIGIN=$portal_origin
PORTAL_TLS_MODE=managed
PORTAL_ACME_LISTEN=$portal_acme_listen
PORTAL_DATA_DIR=$portal_data
PORTAL_KEY_FILE=$portal_config/secrets/master.key
PORTAL_WEB_DIR=$portal_install/web
GOMEMLIMIT=40MiB
EOF
chmod 0640 "$portal_config/portal.env"
chown root:portal-svc "$portal_config/portal.env"
portal_exec() {
  case "$1" in init|tls-config|tls-issue) ;; *) return 1;; esac
  env PORTAL_ORIGIN="$portal_origin" PORTAL_DATA_DIR="$portal_data" PORTAL_KEY_FILE="$portal_config/secrets/master.key" PORTAL_ACME_LISTEN="$portal_acme_listen" su -s /bin/sh -p -c "exec /opt/3x-ui-portal/portal $1" portal-svc
}
portal_exec init
if [[ "$portal_mode" == http || "$portal_mode" == cloudflare ]]; then
  echo "HTTP-01 需要公网 80 到 $portal_acme_listen；任意 NAT 高端口不能代替公网 80。"
  read -rp 'ACME 联系邮箱：' portal_email
  echo '请阅读 https://letsencrypt.org/repository/ 的订阅者协议。'
  read -rp '同意协议并申请证书？输入 yes：' portal_agree
  [[ "$portal_agree" == yes ]] || { echo '配置已保留。请按手册完成证书申请。'; exit 1; }
  portal_dns_token=''
  if [[ "$portal_mode" == cloudflare ]]; then read -rsp 'Cloudflare API Token（不会回显）：' portal_dns_token; echo; fi
  [[ "$portal_email" =~ ^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+$ ]] || { echo '邮箱格式不受支持，请用 CLI JSON 配置'; exit 1; }
  [[ "$portal_dns_token" =~ ^[A-Za-z0-9_-]*$ ]] || { echo 'DNS Token 格式无效'; exit 1; }
  printf '{"email":"%s","method":"%s","dnsToken":"%s","agreeTerms":true,"staging":false}\n' "$portal_email" "$portal_mode" "$portal_dns_token" | portal_exec tls-config
  unset portal_dns_token
  if [[ "$portal_init" == systemd ]]; then
    systemd-run --quiet --wait --pipe --collect --unit="portal-cert-bootstrap-$$" -p User=portal-svc -p Group=portal-svc -p "EnvironmentFile=$portal_config/portal.env" -p AmbientCapabilities=CAP_NET_BIND_SERVICE "$portal_install/portal" tls-issue
  else portal_exec tls-issue; fi
else
  if [[ "$portal_mode" == test-self-signed ]]; then
    echo '仅测试：生成自签名证书，不会被浏览器或订阅客户端自动信任。'
    if [[ "$portal_host" == *:* || "$portal_host" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then portal_san="IP:$portal_host"; else portal_san="DNS:$portal_host"; fi
    openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 7 -subj '/CN=Portal test certificate' -addext "subjectAltName=$portal_san" -keyout "$portal_tmp/key.pem" -out "$portal_tmp/cert.pem" >/dev/null 2>&1
    portal_cert="$portal_tmp/cert.pem"; portal_key="$portal_tmp/key.pem"
  else
    read -rp '完整证书链 PEM 路径：' portal_cert
    read -rp '证书私钥 PEM 路径：' portal_key
  fi
  install -m 0600 -o portal-svc -g portal-svc "$portal_cert" "$portal_data/import-cert.pem"
  install -m 0600 -o portal-svc -g portal-svc "$portal_key" "$portal_data/import-key.pem"
  env PORTAL_ORIGIN="$portal_origin" PORTAL_DATA_DIR="$portal_data" PORTAL_KEY_FILE="$portal_config/secrets/master.key" PORTAL_IMPORT_CERT="$portal_data/import-cert.pem" PORTAL_IMPORT_KEY="$portal_data/import-key.pem" su -s /bin/sh -p -c 'exec /opt/3x-ui-portal/portal tls-import' portal-svc
  rm -f "$portal_data/import-cert.pem" "$portal_data/import-key.pem"
fi
chown -R root:portal-svc "$portal_config"
chmod 0640 "$portal_config/secrets/master.key"
if [[ "$portal_init" == systemd ]]; then
  install -m 0644 "$portal_tmp/deploy/3x-ui-portal.service" /etc/systemd/system/3x-ui-portal.service
  systemctl daemon-reload
  systemctl enable --now 3x-ui-portal.service
  systemctl is-active --quiet 3x-ui-portal.service
else
  install -m 0755 "$portal_tmp/deploy/3x-ui-portal.openrc" /etc/init.d/3x-ui-portal
  rc-update add 3x-ui-portal default
  rc-service 3x-ui-portal start
  rc-service 3x-ui-portal status
fi
printf '服务已运行：%s\n读取一次性安装凭证：sudo cat %s/setup.token\n' "$portal_origin" "$portal_data"
printf '打开 %s/install/安装凭证/，设置后缀、账号密码和面板密钥。\n' "$portal_origin"
