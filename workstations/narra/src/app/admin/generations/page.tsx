import { stat } from 'node:fs/promises';
import { requireAdminRecord } from '@/lib/server/current-user';
import { db } from '@/lib/db';
import { mediaParts } from '@/lib/workstation/files';
export const dynamic = 'force-dynamic';
const statusLabels = { PENDING: '排队中', PROCESSING: '生成中', SUCCEEDED: '成功', FAILED: '失败' };
export default async function AdminGenerations() {
  try { await requireAdminRecord(); } catch { return <main className="shell">无权访问</main>; }
  const [records, tasks] = await Promise.all([
    db.generationImage.findMany({
      where: { job: { user: { createdAt: { gt: new Date(Date.now() - 21600000) } } } },
      include: { job: { select: { prompt: true, userId: true, createdAt: true, model: true } } },
      take: 200, orderBy: { createdAt: 'desc' },
    }),
    db.generationJob.findMany({
      where: { createdAt: { gt: new Date(Date.now() - 86400000) } },
      select: { id: true, model: true, status: true, errorMessage: true, createdAt: true, user: { select: { createdAt: true } } },
      take: 50, orderBy: { createdAt: 'desc' },
    }),
  ]);
  const images = [];
  for (const image of records) {
    try { await stat(mediaParts(image.job.userId, image.url.split('/').pop()!)); images.push(image); } catch { /* Already cleaned. */ }
  }
  return <main className="shell">
    <header><h1>当前未清理图片</h1><a href="/image-workstation/create">返回创作台</a></header>
    <p>图片保留 6 小时，最多展示 200 张。刷新查看最新结果。</p>
    <div className="site-gallery">{images.map(i => <article key={i.id}>
      <a href={i.url} target="_blank" rel="noreferrer"><img src={i.url} alt={i.job.prompt} loading="lazy" /></a>
      <p>{i.job.model}</p><p>{i.job.prompt}</p><time>{i.createdAt.toISOString()}</time>
    </article>)}</div>
    {!images.length && <p>暂无未清理的生成图片。</p>}
    <h2 style={{ fontSize: 22, marginTop: 32 }}>最近任务</h2>
    <p>最近 24 小时的 50 个任务。图片到期后清理，任务记录保留至 24 小时。</p>
    <div style={{ overflowX: 'auto' }}><table style={{ width: '100%', minWidth: 600, textAlign: 'left', borderCollapse: 'collapse' }}>
      <thead><tr><th>时间</th><th>模型</th><th>状态</th><th>图片保留</th><th>失败原因</th></tr></thead>
      <tbody>{tasks.map(t => <tr key={t.id} style={{ borderTop: '1px solid #ded0c3' }}>
        <td style={{ padding: '12px 0' }}>{t.createdAt.toISOString()}</td><td>{t.model}</td><td>{statusLabels[t.status]}</td>
        <td>{Date.now() - t.user.createdAt.getTime() < 21600000 ? '会话有效期内' : '已到清理时间'}</td>
        <td style={{ maxWidth: 300, overflowWrap: 'anywhere' }}>{t.errorMessage?.replace(/https?:\/\/[^\s"']+/g, '[图片下载地址]') || '—'}</td>
      </tr>)}</tbody>
    </table></div>
  </main>;
}
