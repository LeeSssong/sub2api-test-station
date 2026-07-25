import { Check, CircleDot, Cpu, EyeOff, Globe2, LockKeyhole, Network, ShieldCheck, Sparkles } from 'lucide-react'
import { Reveal } from '../components/Reveal'
import type { SiteConfig } from '../domain/siteConfig'
import { ModelFlow } from './ModelFlow'

interface ValueSectionsProps {
  config: SiteConfig
}

export function ValueSections({ config: _config }: ValueSectionsProps) {
  return (
    <section className="value-band grid-surface" id="value" aria-labelledby="value-title">
      <div className="section-inner">
        <Reveal as="header" className="section-intro">
          <p className="eyebrow"><span />全球模型 · 一座星桥</p>
          <h2 id="value-title">国内直连、透明价格、真实模型</h2>
          <p>不绕远路，不隐藏折扣。每一条承诺都写清楚边界。</p>
        </Reveal>

        <Reveal className="value-grid">
          <article className="value-card value-card--wide model-card">
            <div className="card-heading">
              <Network aria-hidden="true" />
              <div><h3>一条 API，接住所有模型</h3><p>请求通过星桥网关路由到健康上游，接口与官方 SDK 保持兼容。</p></div>
            </div>
            <ModelFlow />
          </article>

          <article className="value-card value-card--wide security-card">
            <div className="card-heading">
              <ShieldCheck aria-hidden="true" />
              <div><h3>安全与透明</h3><p>传输、隐私与模型真实性，都给出明确的公开说明。</p></div>
            </div>
            <div className="security-proof-grid">
              <div className="security-proof">
                <LockKeyhole aria-hidden="true" />
                <span><strong>HTTPS 加密传输</strong><small>API 请求全程通过 TLS 加密，密钥不会写入首页配置。</small></span>
              </div>
              <div className="security-proof">
                <EyeOff aria-hidden="true" />
                <span><strong>无第三方追踪</strong><small>首页不加载第三方脚本、追踪器或远程字体。</small></span>
              </div>
              <div className="security-proof security-proof--report">
                <ShieldCheck aria-hidden="true" />
                <span><strong>已获得 MODELOC 真实性验证</strong><small>模型真实性已通过第三方验证。</small><b className="report-status report-status--verified">已验证</b></span>
              </div>
            </div>
          </article>

          <article className="value-card price-card">
            <div className="card-heading"><Sparkles aria-hidden="true" /><div><h3>透明定价</h3><p>公开展示，不用复杂换算掩盖实际折扣。</p></div></div>
            <dl className="price-table">
              <div><dt>星桥价格</dt><dd>官方价格的 0.1—0.3 倍</dd><span className="sr-only">官方价格的 0.1—0.3 倍</span></div>
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

          <article className="value-card value-card--wide network-card">
            <div className="card-heading"><Globe2 aria-hidden="true" /><div><h3>面向国内的稳定线路</h3><p>星桥，缩短国内用户访问世界模型的网络路径。</p></div></div>
            <ul className="check-list">
              <li><CircleDot aria-hidden="true" />韩国首尔服务器，国内无需翻墙即可直连</li>
              <li><CircleDot aria-hidden="true" />减少跨区域中转与不必要跳数</li>
              <li><CircleDot aria-hidden="true" />优先保障稳定性与响应质量</li>
            </ul>
          </article>
        </Reveal>
      </div>
    </section>
  )
}
