# Keeper 号池额度

在官方 v1.15.10 概览中加入号池卡片。只使用原生鉴权、身份、配额刷新/缓存、价格/倍率规则和请求事件 API；不改票据插件、不改数据库、不接管用户结算。统计所有 CPA API Key 的费用，号池为当前启用且套餐相同的 Codex 周额度身份。必须保持所有模型价格倍率及匹配规则为 1，模拟扣费倍率单独保存。

首次打开卡片建立新基线，基线按浏览器和子路径保存在 localStorage。请求按原生游标分页读取，仅累计基线起点（含）至号池配额共同观察截点（不含）的标准费用；不使用凭证行的 Token/Cost。配额每四分钟主动刷新，缓存每分钟检查；页面隐藏或卸载停止。成员/周期/价格变化、配额回升自动重建基线；缺价格、刷新失败或窗口不完整不发布预测。新基线不导入此前的号池用量。

公式：每账号完整容量 = 同期标准费用 / 各账号剩余比例下降之和；号池总容量 = 每账号容量 × 数量；剩余容量 = 每账号容量 × 当前剩余比例之和。假定容量相近、所有相关业务经过此 CPA，且无外部未采集消耗。模拟扣费 = 采样期标准费用 × 输入倍率，不是实际账本。

source.json 固定官方源码归档、补丁、静态资源校验和。复现：下载并核对官方归档，解压后 git apply upstream.patch；web 内 npm ci --ignore-scripts --no-audit --no-fund、npm run typecheck、npm exec -- vitest run src/components/usage/pool/test/poolMath.test.ts src/components/usage/pool/test/PoolQuotaCard.test.tsx；npm run build -- --base=/keeper/。将 dist/index.html 中字符串 APP_BASE_PATH 占位符替换为 /keeper，打包 dist 为 web.tar.gz。

部署从已推送的干净根目录 main 上传本目录的 web.tar.gz、source.json、upstream.patch，加上 infra/cpa-test-station/Caddyfile 和 ops/deploy-cpa-test-keeper-pool.py。执行 sudo python3 deploy.py <bundle> <main-commit> <main-tree>。先核验固定校验和与现场 Caddy，再解压不可变版本目录、验证 Caddy、原子切换 keeper-current 并平滑 reload；Keeper API、数据库和采集进程不重启。

每次发布只保存一份 /opt/cpa-test-station/backups/keeper-pool-<UTC>/release.json，保留旧 Caddy、旧 UI 和所有数据。失败自动恢复旧 Caddy/链接并 reload；首次回退恢复原生 Keeper 前端。手动回退同样只恢复 Caddy 和版本链接，不回滚业务数据。
