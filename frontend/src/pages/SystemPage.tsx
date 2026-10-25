import type { Activity } from '../api/types'
import { Empty, ErrorBox, Loading } from '../components/States'
import { formatTL, statusLabels } from '../format'

export function SystemPage({ activity, error, reload }: { activity: Activity | null; error: string | null; reload: () => void }) {
  const run = activity?.simulation
  const max = Math.max(1, ...activity?.minutes.map((m) => m.count) ?? [])
  return <section>
    {error && <ErrorBox message={error} onRetry={reload} />}
    {!activity && !error && <Loading text="Sistem durumu yükleniyor…" />}
    {activity && <>
      <p>Veritabanı: {activity.db_ok && !error ? 'Bağlı' : 'Bağlantı hatası'} · Son olay: {activity.last_event_at ? new Date(activity.last_event_at).toLocaleString('tr-TR') : 'henüz yok'}</p>
      <figure className="panel"><figcaption>Son 30 dakika · olay sayıları (en çok {max})</figcaption>
        <div className="minute-bars" role="img" aria-label="Son 30 dakikanın olay sayıları">{activity.minutes.map((m) => <span key={m.at} style={{ height: `${Math.max(2, m.count * 100 / max)}%` }} title={`${new Date(m.at).toLocaleTimeString('tr-TR')}: ${m.count}`} />)}</div>
      </figure>
      {!run ? <Empty text="Henüz simülasyon koşusu yok." /> : <section className="panel">
        <h3>Simülasyon · {run.status}</h3>
        {(run.phase === 'LIVE' || run.phase === 'LIVE_STOPPED') && <p>Özet toplu koşunun sonundandır; canlı işlemleri içermez.</p>}
        <p>Aşama: {run.phase} · {run.done.toLocaleString('tr-TR')} / {run.target.toLocaleString('tr-TR')}</p>
        <progress aria-label="Simülasyon ilerlemesi" value={run.done} max={Math.max(1, run.target)} />
        <p>Tohum: {run.seed} · Başlangıç: {new Date(run.started_at).toLocaleString('tr-TR')}{run.finished_at && ` · Bitiş: ${new Date(run.finished_at).toLocaleString('tr-TR')}`}</p>
        {run.fingerprint && <p className="fingerprint">Parmak izi: {run.fingerprint}</p>}
        {run.summary.policies && <><h4>Poliçeler</h4><ul>{run.summary.policies.map((p) => <li key={`${p.product}-${p.status}`}>{p.product} · {statusLabels[p.status] ?? p.status}: {p.count.toLocaleString('tr-TR')}</li>)}</ul></>}
        {run.summary.claims && <><h4>Hasarlar</h4><ul>{run.summary.claims.map((c) => <li key={c.status}>{statusLabels[c.status] ?? c.status}: {c.count.toLocaleString('tr-TR')} · işaretli {c.flagged}</li>)}</ul></>}
        {run.summary.tasks && <p>Açık görev: {run.summary.tasks.open} · SLA ihlali: {run.summary.tasks.breached}</p>}
        {run.summary.ledger && <p>Yazılan prim: {formatTL(run.summary.ledger.written_premium_kurus)} · Nakit: {formatTL(run.summary.ledger.cash_kurus)} · Tazminat gideri: {formatTL(run.summary.ledger.claim_expense_kurus)}</p>}
        {run.summary.balanced !== undefined && <p>Defter: {run.summary.balanced ? 'Dengede' : 'Dengesiz'} · 1200 mutabakatı: {run.summary.receivable_matches ? 'Sağlandı' : 'Sağlanamadı'}</p>}
      </section>}
    </>}
  </section>
}
