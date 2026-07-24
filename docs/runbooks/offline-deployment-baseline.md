# Sub2API 离线部署基线运行手册

**适用范围：** 在服务器、域名、上游额度、订阅账号和商户渠道均未购买/开通时，验证部署包本身。  
**不代表：** 生产部署、真实 HTTPS、上游可用、计费正确或支付可用。

## 1. 基线内容

- Sub2API v0.1.155、PostgreSQL 18、Redis 8、Caddy 2.10.2 均锁定镜像 digest。
- 只有 Caddy 暴露宿主机 80/443；其他服务只在 Compose 默认网络内通信。
- 2 GiB 假定主机使用 PostgreSQL 60 连接、128 MB shared buffers，应用连接池 20，Redis 连接池 64。
- Caddy 的 Docker 私网来源 `172.16.0.0/12` 被声明为可信代理，文本 MVP 的全局和网关请求体上限均为 16 MiB。
- URL 安全默认值为：启用 allowlist、禁止不安全 HTTP、禁止私网目标。
- `.env.example` 只用于解析验证；真实或本地运行必须生成独立 `.env`。

## 2. 本地验证

### 2.1 前置条件

```bash
docker version
docker compose version
openssl version
rg --version
```

预期：四条命令均退出 0，Docker daemon 可用。

### 2.2 生成本地环境文件

```bash
./ops/generate-env.sh
stat -f '%Lp' infra/.env 2>/dev/null || stat -c '%a' infra/.env
git check-ignore infra/.env
```

预期：创建 `infra/.env`，权限显示 `600`，Git 忽略规则命中。脚本不会覆盖已存在文件；需要重新生成时，先人工确认旧文件不再使用，再自行删除。

### 2.3 静态契约

```bash
bash tests/infra/validate-baseline.sh
docker compose --env-file infra/.env -f infra/compose.yaml config --quiet
```

预期：第一条输出 `PASS: infrastructure baseline contracts`，第二条无输出并退出 0。

### 2.4 镜像和版本

```bash
docker compose --env-file infra/.env -f infra/compose.yaml pull
docker run --rm \
  weishaw/sub2api@sha256:5433a314b1dacce7882d0739a6ec24bdec1419a93fba5a34bdecad950137cbb5 \
  --version
```

预期：版本输出包含 `Sub2API 0.1.155`。若不一致，停止启动，不用该摘要部署。

### 2.5 隔离本地启动

未录入真实上游或支付凭据时，可以把 Caddy 站点地址临时覆盖为本机 HTTP：

```bash
SITE_ADDRESS=http://localhost \
  docker compose --env-file infra/.env -f infra/compose.yaml up -d

docker compose --env-file infra/.env -f infra/compose.yaml ps
curl -fsS http://localhost/health
```

预期：`postgres`、`redis`、`sub2api` 显示 healthy，`caddy` 显示 running，健康接口返回成功。若本机 80/443 已被占用，不停止现有服务；跳过本地启动或使用不提交的 Compose override 改为其他回环端口。

### 2.6 停止本地验证

```bash
SITE_ADDRESS=http://localhost \
  docker compose --env-file infra/.env -f infra/compose.yaml down
```

默认保留命名卷，便于检查重启后的数据持久化。只有确认本地测试数据不再需要时才运行带 `-v` 的清理命令。

## 3. 真实部署前必须替换

- [ ] `SITE_ADDRESS`：真实 `api` 子域，不能保留 `api.example.com`。
- [ ] `ADMIN_EMAIL`：项目管理员邮箱，不能保留 `admin@example.com`。
- [ ] URL 策略与已批准部署目标一致；Sub2API 默认值为 allowlist 关闭、允许 HTTP/私网主机、上游域名名单为空。
- [ ] 五个密钥均由 `ops/generate-env.sh` 生成，`.env` 权限为 600。
- [ ] SRV01 已实际购买并登记实例 ID、公网 IP、区域、到期日和自动续费状态。
- [ ] Ubuntu 24.04 LTS 已重装或确认无未知预装环境。
- [ ] 云防火墙只开放 80/443；22 只允许管理来源；数据库、Redis 和 8080 不公网开放。
- [ ] 2 GiB 主机已建立 2 GiB swap，`swapon --show` 可见。
- [ ] `vm.overcommit_memory=1` 已生效，Redis 启动日志不再出现内存 overcommit 警告。
- [ ] DNS A 记录指向实例公网 IP，TTL 300 秒，Cloudflare 首版为 DNS only。
- [ ] Caddy 已成功取得真实域名证书，HTTP 自动跳转 HTTPS。
- [ ] 管理员首次登录后修改邮箱/密码并启用 2FA。

2 GiB swap 的真实主机命令仅在服务器购买并确认磁盘空间后执行：

```bash
sudo fallocate -l 2G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
echo 'vm.overcommit_memory = 1' | sudo tee /etc/sysctl.d/99-sub2api.conf
sudo sysctl --system
swapon --show
sysctl vm.overcommit_memory
```

执行前先确认 `/etc/fstab` 中没有重复 `/swapfile` 条目。预期最后一条显示 `vm.overcommit_memory = 1`。

## 4. 真实环境复验

资产到位后按顺序验证：

1. 服务器出站能访问现有上游域名，且运行时 URL 策略与 `.env` 和 Compose 中的批准值一致。
2. 国内外 DNS 都解析到当前实例；80/443 可达，22 的来源限制生效。
3. `docker compose ps` 四个服务状态正常，重启后数据和管理员会话所需固定密钥仍有效。
4. 用管理员测试 Key 分别发送非流式和流式请求；检查 SSE 不被缓冲或中断。
5. 在本站和上游两端核对请求 ID、模型、Token、费用和余额变化。
6. 验证余额不足、Key 吊销、上游 429/5xx、客户端取消和超时路径。

## 5. 当前明确未验证

- 服务器未购买，SSH、防火墙、swap 和境外网络未验证。
- 域名未确认/购买，DNS 未修改，真实证书未申请。
- 上游非敏感参数尚未盘点，API Key 未录入，未发送真实模型请求。
- 日志、Token 计量、成本和余额扣费尚未经过真实闭环。
- 未接收真实客户款项，未配置人工充值实单、商户渠道或支付密钥。
- K12、Plus、Pro 等订阅账号未购买、未授权、未接入。

以上项目只有在真实资产和明确授权到位后才能改为“已验证”。
