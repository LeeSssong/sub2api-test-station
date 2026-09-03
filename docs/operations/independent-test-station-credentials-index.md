# 独立测试站凭据索引

本文件不保存秘密值。读取前确认权限为 0600；秘密只通过受保护文件或系统 keyring 使用。

| 用途 | 位置/来源 | 验证 |
|---|---|---|
| GitHub 私有仓库 | `gh auth` 系统 keyring | `gh auth status` |
| 测试服务器 SSH 私钥 | `/Users/gongtengxinwen/.ssh/tencent_lighthouse_seoul_sub2api` | `stat -f '%Sp %N' ...` 应为 `-rw-------` |
| SSH 主机指纹 | `/Users/gongtengxinwen/.config/sub2api/known_hosts` | SSH 使用 `StrictHostKeyChecking=yes` |
| 测试站运行 env | 服务器 `/opt/sub2api-test-station/.env` | 服务器端 `stat` 应为 0600；禁止输出内容 |
| 旧验收 env（仅历史兼容） | `/Users/gongtengxinwen/.config/sub2api/acceptance-20260827.env` | 不得用于独立测试站 Compose |

允许展示变量名、是否设置和文件权限；禁止展示变量值、整行 env、Authorization、Bearer、API key、密码、私钥内容或 token。
