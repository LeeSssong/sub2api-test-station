import { Gauge, Scale, TriangleAlert } from 'lucide-react'

const boundaries = [
  {
    icon: Gauge,
    title: '容量保护',
    lead: '现有用户的稳定优先于无限扩张。',
    items: ['高负载时可能降低并发并补充上游容量。', '达到容量边界时可能暂停注册或转为邀请制。', '不会为了吞吐量牺牲账号与通道稳定。'],
  },
  {
    icon: Scale,
    title: '合理使用',
    lead: '正常交互使用被支持，刷量不属于正常使用。',
    items: ['支持编码工具与模型的日常真实调用。', '自动消耗额度、人工压测与滥用脚本并发可能被限制。', '处理方式逐步升级，并通过 QQ 群公开沟通。'],
  },
  {
    icon: TriangleAlert,
    title: '事件与责任',
    lead: '把可控范围与上游风险分开说明。',
    items: ['官方宕机、政策变化和上游账号处置不由星桥直接控制。', '已知事件会尽快沟通，并在可用时切换容量。', '不承诺绝对可用率、退款或服务补偿。'],
  },
]

export function BoundarySection() {
  return (
    <section className="boundary-band" id="about" aria-labelledby="boundary-title" tabIndex={-1}>
      <div className="section-inner">
        <header className="section-intro section-intro--light">
          <p className="eyebrow"><span />服务边界</p>
          <h2 id="boundary-title">边界清晰，承诺才有意义</h2>
          <p>商业服务需要明确规则。这里写清楚我们保障什么，也写清楚哪些风险不在星桥控制范围内。</p>
        </header>
        <div className="boundary-grid">
          {boundaries.map(({ icon: Icon, title, lead, items }, index) => (
            <article key={title}>
              <div className="boundary-index">0{index + 1}</div>
              <Icon aria-hidden="true" />
              <h3>{title}</h3>
              <p>{lead}</p>
              <ul>{items.map((item) => <li key={item}>{item}</li>)}</ul>
            </article>
          ))}
        </div>
      </div>
    </section>
  )
}
