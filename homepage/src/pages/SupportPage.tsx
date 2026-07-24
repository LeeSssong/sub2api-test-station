import { CopyControl } from '../components/CopyControl'

export function SupportPage({ qqGroup }: { qqGroup: string }) {
  return (
    <main className="support-page">
      <section className="support-panel" aria-labelledby="support-title">
        <div className="support-copy">
          <p className="support-kicker">XINGQIAO SUPPORT</p>
          <h1 id="support-title">联系客服</h1>
          <p>扫描二维码加入 QQ 群，或复制群号手动搜索。</p>
          <div className="support-number-row">
            <span>QQ群号</span><strong>{qqGroup}</strong>
            <CopyControl value={qqGroup} label="复制 QQ 群号" />
          </div>
        </div>
        <figure className="support-qr">
          <img src="/support/qq-group-1080152144.png" alt={`QQ群 ${qqGroup} 二维码`} />
        </figure>
      </section>
    </main>
  )
}
