# 本地提链支付工作台

本工具在本机维护 miaodongAI API 会话，按每行 `Access Token`、提链 CDK、支付 CDK 的 1:1:1 对应关系执行提链和支付，并将批次历史与任务状态保存到 SQLite。

## 运行

需要 Node.js 22.5 或更高版本。

```bash
cd tools/local-workbench
npm start
```

打开 `http://127.0.0.1:4318`。

默认数据库为 `data/workbench.sqlite`。修改位置或端口：

```bash
WORKBENCH_DB=/absolute/path/workbench.sqlite PORT=4320 npm start
```

## 使用

在输入表中逐行填写，或从 Excel 粘贴三列 TSV 数据：

```text
access_token_1<TAB>extract_cdk_1<TAB>payment_cdk_1
access_token_2<TAB>extract_cdk_2<TAB>payment_cdk_2
```

每行必须恰好三列。点击“启动任务”后，服务会：

1. 批量提交提链任务并轮询结果。
2. 按支付 CDK 分组提交已提取的 HTTPS 支付链接。
3. 自动轮询支付状态，直到成功、失败或轮询超时。

停止后重新运行会恢复未完成批次。支付提交发生网络结果未知时，任务会标记为失败并提示核对，避免自动重复支付。

## 验证

```bash
npm test
```

接口依据：`https://kk.642636.xyz/api-docs`
