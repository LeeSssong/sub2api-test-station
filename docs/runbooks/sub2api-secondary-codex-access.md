# 第二终端 Codex 服务器接入

这份文件只提供接入流程和非秘密连接参数。第二终端必须拥有单独的私钥文件；私钥不会进入 Git、项目目录、聊天记录或本文件。

## 已配置的远程入口

- 主机：43.133.75.82
- SSH 端口：2222
- 用户：ubuntu
- 远程工作目录：/opt/sub2api/production
- Compose 项目：sub2api
- 远程主机 ED25519 指纹：SHA256:6lH9yxzWyCfxEejPf9J/0HOig4w6W8kTkaYJbYIziUM
- 本次独立公钥指纹：SHA256:86yMd8CVGzxPXofXEbI/QosE5UTY91gKEP9hjS834ww

## 在第二终端准备

推荐直接通过安全的离线介质转移本机 bundle：

    /Users/gongtengxinwen/.config/sub2api/codex-secondary-access-20260824-portable.tgz

SHA-256：7415b8db577f804c50e7e5f515309e3e8c27fb455fa66002fd4226394ff5d635

在第二终端解压后，bundle 内的私钥会位于 sub2api-secondary-access/.ssh/；将其移动到下方指定位置并按命令设置权限。不要通过聊天发送 bundle。

把本机受限目录中的两个文件通过安全的离线方式复制到第二终端（不要粘贴到聊天或提交到 Git）：

    /Users/gongtengxinwen/.ssh/sub2api_codex_secondary_20260824
    /Users/gongtengxinwen/.config/sub2api/known_hosts

第二终端保存为：

    ~/.ssh/sub2api_codex_secondary_20260824
    ~/.config/sub2api/known_hosts

然后执行：

    chmod 700 ~/.ssh ~/.config/sub2api
    chmod 600 ~/.ssh/sub2api_codex_secondary_20260824 ~/.config/sub2api/known_hosts
    ssh -F /dev/null -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile=~/.config/sub2api/known_hosts -i ~/.ssh/sub2api_codex_secondary_20260824 -p 2222 ubuntu@43.133.75.82 'whoami; id; sudo -n -l'

预期用户为 ubuntu，并且 sudo -n 可用。第二终端 Codex 的 SSH 主机配置：

    Host sub2api-prod-secondary
        HostName 43.133.75.82
        Port 2222
        User ubuntu
        IdentityFile ~/.ssh/sub2api_codex_secondary_20260824
        IdentitiesOnly yes
        UserKnownHostsFile ~/.config/sub2api/known_hosts
        StrictHostKeyChecking yes
        ForwardAgent no
        ServerAliveInterval 30

## Codex 读取提示

将下面这段作为第二终端 Codex 的本地任务上下文；它不包含私钥：

    Use SSH host alias sub2api-prod-secondary.
    Connect as ubuntu on 43.133.75.82:2222 using the local IdentityFile
    ~/.ssh/sub2api_codex_secondary_20260824 and UserKnownHostsFile
    ~/.config/sub2api/known_hosts. Require StrictHostKeyChecking=yes and
    IdentitiesOnly=yes. Remote workdir is /opt/sub2api/production; Compose project is
    sub2api. Before any write, run: uname -s; docker context show; verify DOCKER_HOST
    is empty; pwd -P; sudo -n true. Do not print or copy private keys, environment
    files, API keys, passwords, or tokens.

## 吊销

需要撤销第二终端时，在已授权终端执行：

    ssh sub2api-prod "sed -i.bak '/codex-secondary-sub2api-20260824$/d' ~/.ssh/authorized_keys"

随后删除第二终端的私钥，并在本机删除对应私钥、本机 manifest 和 bundle。
