import { useEffect, useState } from 'react'
import { api, type Role } from '../api/client'
import type { ClaimDetail, SettlementPreview } from '../api/types'
import { errorText, formatTL, newIdempotencyKey, statusLabels } from '../format'
import { AppraisalForm } from './ClaimForms'
import { Empty, ErrorBox, Loading } from './States'
import { TaskTable } from './TaskTable'

export function ClaimDetailLoader({ id, role, onChanged }: { id: number; role: Role; onChanged?: () => void }) {
  const [claim, setClaim] = useState<ClaimDetail | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let cancelled = false
    api<ClaimDetail>(`/api/v1/claims/${id}`).then((c) => { if (!cancelled) { setClaim(c); setError(null) } }).catch((e: unknown) => !cancelled && setError(errorText(e)))
    return () => { cancelled = true }
  }, [id, attempt])
  if (error) return <ErrorBox message={error} onRetry={() => setAttempt((a) => a + 1)} />
  if (!claim || claim.id !== id) return <Loading text="Hasar dosyası yükleniyor…" />
  return <ClaimView key={`${id}-${attempt}`} claim={claim} role={role} reload={() => { setAttempt((a) => a + 1); onChanged?.() }} />
}
function ClaimView({ claim: c, role, reload }: { claim: ClaimDetail; role: Role; reload: () => void }) {
  const [note, setNote] = useState('')
  const [preview, setPreview] = useState<SettlementPreview | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [approveKey] = useState(newIdempotencyKey)
  const [payKey] = useState(newIdempotencyKey)
  const path = `/api/v1/claims/${c.id}`
  async function act(endpoint: string, body?: unknown, key?: string) {
    setBusy(true); setError(null)
    try {
      const result = await api<SettlementPreview>(path + endpoint, { method: 'POST', ...(key ? { headers: { 'Idempotency-Key': key } } : {}), ...(body ? { body: JSON.stringify(body) } : {}) })
      if (endpoint === '/settlement/preview') setPreview(result)
      else reload()
    } catch (e) { setError(errorText(e)) } finally { setBusy(false) }
  }
  return <section className="panel">
    <h3>{c.claim_no} · {statusLabels[c.status]} · sürüm {c.row_version}</h3>
    <dl className="profile"><dt>Poliçe</dt><dd>{c.policy_no}</dd><dt>Hasar tarihi</dt><dd>{c.loss_date}</dd><dt>Teminat / plaka</dt><dd>{c.coverage_code} / {c.plate ?? '—'}</dd><dt>Talep</dt><dd>{formatTL(c.claimed_amount_kurus)}</dd><dt>Açıklama</dt><dd>{c.description}</dd></dl>
    {c.fraud && <section className="panel"><h4>Fraud sinyalleri (kurgusal)</h4><p>Skor: <strong>{c.fraud.score} / {c.fraud.threshold}</strong> · {c.fraud.flagged ? 'İnceleme eşiğinde' : 'Eşik altında'}</p>{c.fraud.signals.length === 0 ? <Empty text="Sinyal yok." /> : <ul>{c.fraud.signals.map((s) => <li key={s.rule_code}><strong>{s.rule_code} · {s.points} puan</strong> — {s.explanation}</li>)}</ul>}</section>}
    {c.tasks && <><h4>Görevler</h4>{c.tasks.length ? <TaskTable tasks={c.tasks} /> : <Empty text="Görev yok." />}</>}
    <h4>Durum geçmişi</h4>
    <ol>{c.history.map((h) => <li key={h.id}>{new Date(h.created_at).toLocaleString('tr-TR')} · {h.from_status ? `${statusLabels[h.from_status]} → ` : ''}{statusLabels[h.to_status]} {h.note ? `· ${h.note}` : ''}</li>)}</ol>
    <h4>Ekspertizler</h4>
    {c.appraisals.length === 0 ? <Empty text="Henüz ekspertiz yok." /> : <ul>{c.appraisals.map((a) => <li key={a.id}>#{a.id} · {new Date(a.inspected_at).toLocaleString('tr-TR')} · {formatTL(a.damage_amount_kurus)} · {a.is_total_loss ? 'Tam hasar' : 'Kısmi hasar'} · {a.notes}</li>)}</ul>}
    {role === 'APPRAISER' && c.status === 'APPRAISAL' && <AppraisalForm claimID={c.id} onDone={reload} />}
    {role === 'CLAIMS_ADJUSTER' && (c.allowed_transitions?.length ?? 0) > 0 && <div>
      <label>Geçiş notu (red ve fraud incelemesi için zorunlu)<textarea maxLength={4000} value={note} onChange={(e) => setNote(e.target.value)} disabled={busy} /></label>
      <small>Red gerekçesi müşteriye gösterilir. İnceleme notları iç kullanım içindir.</small>
      <div className="actions">{c.allowed_transitions?.map((to) => <button key={to} disabled={busy || (to === 'ADJUSTMENT' && c.appraisals.length === 0) || (['REJECTED', 'FRAUD_REVIEW'].includes(to) && !note.trim())} onClick={() => void act('/transition', { to_status: to, note, expected_row_version: c.row_version })}>{statusLabels[to]}</button>)}</div>
    </div>}
    {role === 'CLAIMS_ADJUSTER' && c.status === 'ADJUSTMENT' && <div className="actions"><button disabled={busy} onClick={() => void act('/settlement/preview')}>Tazminat önizle</button></div>}
    {(preview || c.settlement) && <SettlementSteps result={(c.settlement ?? preview)!} />}
    {preview && role === 'CLAIMS_ADJUSTER' && c.status === 'ADJUSTMENT' && <button disabled={busy || preview.payable_kurus === 0} onClick={() => void act('/settlement/approve', undefined, approveKey)}>Tazminatı onayla</button>}
    {role === 'ACCOUNTING' && c.status === 'APPROVED' && <button disabled={busy} onClick={() => void act('/pay', undefined, payKey)}>Öde</button>}
    {c.settlement?.paid_at && <p>Ödendi: {new Date(c.settlement.paid_at).toLocaleString('tr-TR')} · Yevmiye #{c.settlement.payment_journal_entry_id}</p>}
    {error && <ErrorBox message={error} />}
    {busy && <Loading text="İşlem sürüyor…" />}
  </section>
}
function SettlementSteps({ result }: { result: SettlementPreview }) {
  return <div><h4>Tazminat hesabı</h4><ol>{result.calc_steps.map((s) => <li key={s.code}>{s.description} → <strong>{formatTL(s.output_kurus)}</strong>{s.clipped_kurus > 0 ? ` (azalan: ${formatTL(s.clipped_kurus)})` : ''}</li>)}</ol><p>Ödenecek: <strong>{formatTL(result.payable_kurus)}</strong></p></div>
}
