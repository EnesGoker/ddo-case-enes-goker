import { useEffect, useState, type FormEvent } from 'react'
import { api, type Role } from '../api/client'
import type { BillingPlan } from '../api/types'
import { errorText, formatTL, newIdempotencyKey, todayISO } from '../format'
import { Empty, ErrorBox, Loading } from './States'
import { StatusBadge } from './StatusBadge'

const labels: Record<string, string> = { PENDING: 'Bekliyor', PARTIAL: 'Kısmen ödendi', PAID: 'Ödendi', CANCELLED: 'İptal' }

export function BillingPanel({ policyID, role }: { policyID: number; role: Role }) {
  const [plan, setPlan] = useState<BillingPlan | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)
  const [refundID, setRefundID] = useState<number | null>(null)
  const reload = () => { setPlan(null); setError(null); setAttempt((a) => a + 1) }

  useEffect(() => {
    let cancelled = false
    api<BillingPlan>(`/api/v1/policies/${policyID}/billing`)
      .then((p) => { if (!cancelled) setPlan(p) })
      .catch((e: unknown) => { if (!cancelled) setError(errorText(e)) })
    return () => { cancelled = true }
  }, [policyID, attempt])

  return <section className="panel">
    <h3>Ödeme planı</h3>
    {error && <ErrorBox message={error} onRetry={reload} />}
    {!error && !plan && <Loading />}
    {plan && <>
      <div className="financial-summary"><p>Plan toplamı: {formatTL(plan.total_kurus)}<br />Açık bakiye</p><strong>{formatTL(plan.open_balance_kurus)}</strong></div>
      <p className="muted">Günlük basit faiz: {plan.daily_rate_ppm} ppm (kurgusal). Açık bakiye son tahakkuka göredir; ödeme sırasında faiz güncellenir.</p>
      <div className="table-wrap"><table><thead><tr><th>Taksit / vade</th><th className="num">Tutar</th><th className="num">Ödenen</th><th>Durum / gecikme</th></tr></thead><tbody>
        {plan.installments.map((i) => <tr key={i.id}>
          <td data-label="Taksit / vade">#{i.seq} · {i.due_date}</td>
          <td data-label="Tutar" className="num">{formatTL(i.amount_kurus)}</td>
          <td data-label="Ödenen" className="num">{formatTL(i.paid_kurus)}</td>
          <td data-label="Durum / gecikme"><StatusBadge status={i.status} label={labels[i.status] ?? i.status} /> · {i.overdue_days} gün</td>
        </tr>)}
      </tbody></table></div>
      <h4>Faiz kayıtları</h4>
      {plan.interest.length === 0 ? <Empty text="Henüz faiz tahakkuku yok." /> : <div className="table-wrap"><table><thead><tr><th>Kayıt / taksit</th><th>Aralık [başlangıç, bitiş)</th><th>Anapara / ppm</th><th>Faiz / ödenen</th></tr></thead><tbody>
        {plan.interest.map((c) => <tr key={c.id}>
          <td data-label="Kayıt / taksit">#{c.id} / #{c.installment_id}</td>
          <td data-label="Aralık">{c.accrued_from} → {c.accrued_to}</td>
          <td data-label="Anapara / ppm">{formatTL(c.overdue_principal_kurus)} / {c.daily_rate_ppm}</td>
          <td data-label="Faiz / ödenen">{formatTL(c.amount_kurus)} / {formatTL(c.paid_kurus)}</td>
        </tr>)}
      </tbody></table></div>}
      <h4>Ödemeler ve iadeler</h4>
      {plan.payments.length === 0 ? <Empty text="Henüz ödeme yok." /> : plan.payments.map((p) => <details key={p.id} className="endorsement">
        <summary>#{p.id} · {p.kind === 'PAYMENT' ? 'Tahsilat' : 'İade'} · {formatTL(p.amount_kurus)} · {p.paid_at.slice(0, 10)}</summary>
        <p>{p.method} · {p.reference}{p.refund_of_payment_id ? ` · Ödeme #${p.refund_of_payment_id}` : ''}</p>
        <ul>{plan.allocations.filter((a) => a.payment_id === p.id).map((a) => <li key={a.id}>
          {a.late_interest_charge_id ? `Faiz #${a.late_interest_charge_id}` : `Taksit #${a.installment_id}`}: {formatTL(a.amount_kurus)} {p.kind === 'REFUND' ? '(geri alındı)' : ''}
        </li>)}</ul>
        {p.kind === 'REFUND' && !plan.allocations.some((a) => a.payment_id === p.id) && <p>Prim azalışından nakit iade.</p>}
        {role === 'ACCOUNTING' && plan.policy_status === 'ACTIVE' && p.refundable_kurus > 0 && <>
          <button className="secondary" onClick={() => setRefundID(refundID === p.id ? null : p.id)}>İade et</button>
          {refundID === p.id && <MoneyForm key={p.id} max={p.refundable_kurus} refundID={p.id} role={role} onDone={() => { setRefundID(null); reload() }} />}
        </>}
      </details>)}
      {(role === 'ACCOUNTING' || role === 'CUSTOMER') && plan.open_balance_kurus > 0 && <>
        <h4>{role === 'CUSTOMER' ? 'Öde' : 'Ödeme al'}</h4>
        <MoneyForm policyID={policyID} role={role} onDone={reload} />
      </>}
      <button className="secondary" onClick={reload}>Planı yenile</button>
    </>}
  </section>
}

function MoneyForm({ policyID, refundID, max, role, onDone }: { policyID?: number; refundID?: number; max?: number; role: Role; onDone: () => void }) {
  const [amount, setAmount] = useState('1')
  const [method, setMethod] = useState('TRANSFER')
  const [date, setDate] = useState(todayISO())
  const [reason, setReason] = useState('')
  const [key, setKey] = useState(newIdempotencyKey)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const path = refundID ? `/api/v1/payments/${refundID}/refund` : `/api/v1/policies/${policyID}/payments`
      const body = refundID ? { amount_kurus: Number(amount), reason } : { amount_kurus: Number(amount), method, ...(role === 'ACCOUNTING' ? { payment_date: date } : {}) }
      await api(path, { method: 'POST', headers: { 'Idempotency-Key': key }, body: JSON.stringify(body) })
      onDone()
    } catch (e) {
      setError(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  return <form className="form-grid" onSubmit={(e) => void submit(e)} onChange={() => setKey(newIdempotencyKey())}>
    <label>Tutar (kuruş)<input type="number" required min="1" step="1" max={max ?? Number.MAX_SAFE_INTEGER} value={amount} onChange={(e) => setAmount(e.target.value)} disabled={busy} /></label>
    {refundID ? <label>İade gerekçesi<textarea required maxLength={1000} value={reason} onChange={(e) => setReason(e.target.value)} disabled={busy} /></label> : <>
      <label>Yöntem<select value={method} onChange={(e) => setMethod(e.target.value)} disabled={busy}><option value="TRANSFER">Havale</option><option value="CARD">Kart</option><option value="CASH">Nakit</option></select></label>
      {role === 'ACCOUNTING' && <label>Ödeme tarihi<input type="date" required max={todayISO()} value={date} onChange={(e) => setDate(e.target.value)} disabled={busy} /></label>}
    </>}
    <div className="actions"><button disabled={busy}>{busy ? 'İşleniyor…' : refundID ? 'İadeyi kaydet' : 'Ödemeyi kaydet'}</button></div>
    {error && <ErrorBox message={error} />}
  </form>
}
