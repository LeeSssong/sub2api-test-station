# 支付渠道退款同步确认、可观测性与幂等重试设计

## 问题与证据

Sub 原生退款链同步调用 provider 的 `Refund`。支付宝 `alipay.trade.refund` 返回后，当前适配器只把 `fund_change=Y` 映射为成功，其余结果统一映射为 pending，且丢弃 `code/sub_code/msg/sub_msg` 等响应字段。前端收到 pending 后关闭弹窗；支付宝不实现退款查询接口时，订单进入无法继续操作的状态。

## 目标与非目标

- 保持 Sub 原生同步退款调用链和现有支付渠道请求语义。
- 保留渠道响应的非敏感诊断字段，写入结构化日志和退款审计详情。
- pending 不再伪装成完成；管理员可对 pending/failed 订单重新发起同金额退款。
- 重试使用稳定的订单+金额幂等请求号，避免每次生成时间戳请求号。
- 不新增退款 worker、支付查询接口、迁移或第二账务事实源；不触碰主站。

## 方案

在现有 `RefundRequest/RefundResponse` 增加内部渠道请求号与诊断字段。服务层为同一订单和同一退款金额生成稳定 `out_request_no`，首次和重试均同步调用 provider。支付宝继续调用 `TradeRefund`，仅将请求号从随机时间戳替换为服务层提供的稳定值。pending/failed 的审计详情记录响应字段和阶段；日志脱敏，不记录密钥、签名或完整配置。

前端在 `REFUND_PENDING` 显示“重试退款”，调用独立 retry API；pending 提示后保留订单入口。retry API 复用订单原始退款金额/原因并执行同一同步链。成功转 `REFUNDED`，明确失败转 `REFUND_FAILED`，结果未知保留 `REFUND_PENDING`。

## 验收

1. 支付宝 provider 单测确认稳定请求号传入 `OutRequestNo`，响应诊断字段可见。
2. 服务单测确认同订单同金额生成相同请求号，pending/failed 审计包含诊断字段。
3. 前端测试确认 pending 显示重试入口且调用 retry API，普通成功/失败路径不回归。
4. 在独立测试站使用最新 pending 订单做一次真实重试，记录 HTTP、审计、日志和订单最终状态；不部署主站。
