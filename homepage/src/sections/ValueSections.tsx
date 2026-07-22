import { ArrowUpRight, Check, CircleDot, Copy, Cpu, Globe2, Network, Radio, ShieldCheck, Sparkles } from 'lucide-react'
import { CopyControl } from '../components/CopyControl'
import type { SiteConfig } from '../domain/siteConfig'

interface ValueSectionsProps {
  config: SiteConfig
}

const models = ['OpenAI', 'Claude', 'Gemini', 'DeepSeek', 'Qwen', 'GLM']

export function ValueSections({ config }: ValueSectionsProps) {
  return (
    <section className="value-band grid-surface" id="value" aria-labelledby="value-title">
      <div className="section-inner">
        <header className="section-intro">
          <p className="eyebrow"><span />全球模型 · 一座星桥</p>
          <h2 id="value-title">国内直连、透明价格、真实模型</h2>
          <p>不绕远路，不隐藏倍率。每一条承诺都写清楚边界。</p>
        </header>

        <div className="value-grid">
          <article className="value-card value-card--wide model-card">
            <div className="card-heading">
              <Network aria-hidden="true" />
              <div><h3>一条 API，接住所有模型</h3><p>请求通过星桥网关路由到健康上游，接口与官方 SDK 保持兼容。</p></div>
            </div>
            <div className="model-flow" aria-label="模型路由示意">
              <span className="request-chip"><Radio aria-hidden="true" />你的请求</span>
              <span className="flow-line" aria-hidden="true" />
              <span className="gateway-chip"><img src="/home-assets/xingqiao-logo.png" alt="" />星桥</span>
              <span className="flow-line" aria-hidden="true" />
              <div className="model-chips">{models.map((model) => <span key={model}>{model}</span>)}</div>
            </div>
          </article>

          {config.thirdPartyReports.length > 0 && (
            <article className="value-card value-card--wide report-card">
              <div className="card-heading">
                <ShieldCheck aria-hidden="true" />
                <div><h3>第三方报告</h3><p>独立验证链接，仅在真实报告配置后公开。</p></div>
              </div>
              <div className="report-list">
                {config.thirdPartyReports.map((report) => (
                  <a
                    key={report.id}
                    href={report.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    aria-label={`查看${report.title}`}
                  >
                    <span className="report-provider">{report.provider}</span>
                    <span><strong>{report.title}</strong><small>{report.description ?? '已配置第三方验证报告'}</small></span>
                    <ArrowUpRight aria-hidden="true" />
                  </a>
                ))}
              </div>
            </article>
          )}

          <article className="value-card price-card">
            <div className="card-heading"><Sparkles aria-hidden="true" /><div><h3>透明定价</h3><p>公开展示，不用营销倍率掩盖真实换算。</p></div></div>
            <dl className="price-table">
              <div><dt>官方价格</dt><dd>100%</dd><span className="sr-only">官方价格 100%</span></div>
              <div><dt>星桥价格</dt><dd>官方价格的 0.1–0.3 倍</dd><span className="sr-only">星桥价格 官方价格的 0.1–0.3 倍</span></div>
              <div><dt>额度换算</dt><dd>1 元 = 1 美元额度</dd><span className="sr-only">额度换算 1 元 = 1 美元额度</span></div>
            </dl>
          </article>

          <article className="value-card channel-card">
            <div className="card-heading"><Cpu aria-hidden="true" /><div><h3>真实模型通道</h3><p>OpenAI、Anthropic 及更多主流模型，从同一网关接入。</p></div></div>
            <ul className="check-list">
              <li><Check aria-hidden="true" />OpenAI 与 Anthropic 官方接口格式</li>
              <li><Check aria-hidden="true" />流式响应与工具调用</li>
              <li><Check aria-hidden="true" />模型列表持续更新</li>
            </ul>
          </article>

          <article className="value-card network-card">
            <div className="card-heading"><Globe2 aria-hidden="true" /><div><h3>面向国内的稳定线路</h3><p>韩国首尔节点，缩短国内用户访问世界模型的网络路径。</p></div></div>
            <ul className="check-list">
              <li><CircleDot aria-hidden="true" />韩国首尔服务器，国内无需翻墙即可直连</li>
              <li><CircleDot aria-hidden="true" />减少跨区域中转与不必要跳数</li>
              <li><CircleDot aria-hidden="true" />优先保障稳定性与响应质量</li>
            </ul>
          </article>

          <article className="value-card support-card">
            <div className="card-heading"><Copy aria-hidden="true" /><div><h3>QQ群支持</h3><p>公开支持渠道只保留 QQ 群，反馈路径简单明确。</p></div></div>
            <div className="qq-row"><span>QQ群</span><strong>{config.support.qqGroup}</strong><CopyControl value={config.support.qqGroup} label="复制 QQ 群号" /></div>
          </article>
        </div>
      </div>
    </section>
  )
}
