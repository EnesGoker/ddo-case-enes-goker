import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { Role } from '../api/client'
import type { IssuedPolicy, Quote } from '../api/types'
import { errorText, formatTL, reasonLabels, statusLabels } from '../format'
import { Breakdown } from './Breakdown'
import { ErrorBox, Loading } from './States'

function newKey(): string {
  return typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

// QuoteDetailLoader teklifi (döküm dahil) id ile yükler.
export function QuoteDetailLoader({ id, role, onChanged }: { id: number; role: Role; onChanged?: () => void }) {
  const [quote, setQuote] = useState<Quote | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let cancelled = false
    api<Quote>(`/api/v1/quotes/${id}`)
      .then((q) => {
        if (!cancelled) {
          setQuote(q)
          setError(null)
        }
      })
      .catch((e: unknown) => !cancelled && setError(errorText(e)))
    return () => {
      cancelled = true
    }
  }, [id, attempt])

  if (error) return <ErrorBox message={error} onRetry={() => setAttempt((a) => a + 1)} />
  if (!quote || quote.id !== id) return <Loading text="Teklif yükleniyor…" />
  return (
    <QuoteDetail
      quote={quote}
      role={role}
      onChanged={(q) => {
        setQuote(q)
        onChanged?.()
      }}
    />
  )
}

export function QuoteDetail({ quote, role, onChanged }: { quote: Quote; role: Role; onChanged: (q: Quote) => void }) {
  // Aynı teklifin çevirme denemeleri aynı anahtarı kullanır: ağ hatasında tekrar basmak çift poliçe üretmez.
  const [convertKey] = useState(newKey)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [note, setNote] = useState('')
  const [issued, setIssued] = useState<IssuedPolicy | null>(null)

  const reasons = quote.risk.review_reasons ?? []
  const canConvert = role === 'AGENT' && (quote.status === 'CALCULATED' || quote.status === 'APPROVED')
  const canReview = role === 'UNDERWRITER' && quote.status === 'UW_REVIEW'

  async function run(action: () => Promise<void>) {
    setBusy(true)
    setError(null)
    try {
      await action()
    } catch (e) {
      setError(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  const convert = () =>
    run(async () => {
      const p = await api<IssuedPolicy>(`/api/v1/quotes/${quote.id}/convert`, {
        method: 'POST',
        headers: { 'Idempotency-Key': convertKey },
      })
      setIssued(p)
      onChanged(await api<Quote>(`/api/v1/quotes/${quote.id}`))
    })

  const review = (decision: 'APPROVE' | 'REJECT') =>
    run(async () => {
      onChanged(
        await api<Quote>(`/api/v1/quotes/${quote.id}/review`, {
          method: 'POST',
          body: JSON.stringify({ decision, note }),
        }),
      )
    })

  return (
    <section className="panel">
      <h3>
        Teklif #{quote.id} · {quote.product} · <span className="badge">{statusLabels[quote.status] ?? quote.status}</span>
      </h3>
      <dl className="profile">
        <dt>Sigortalı</dt><dd>{quote.customer_name}</dd>
        <dt>Dönem</dt><dd>{quote.term_start} → {quote.term_end}</dd>
        <dt>Tarife</dt><dd>{quote.product} v{quote.tariff.version_no}</dd>
        {quote.risk.plate && (<><dt>Plaka</dt><dd>{quote.risk.plate}</dd></>)}
        {quote.risk.driver_age !== undefined && (<><dt>Sürücü yaşı</dt><dd>{quote.risk.driver_age}</dd></>)}
        <dt>Girdi özeti</dt><dd className="mono">{quote.input_hash.slice(0, 16)}…</dd>
        {reasons.length > 0 && (<><dt>Onay nedeni</dt><dd>{reasons.map((r) => reasonLabels[r] ?? r).join(', ')}</dd></>)}
        {quote.underwriter_note && (<><dt>UW notu</dt><dd>{quote.underwriter_note}</dd></>)}
      </dl>
      {quote.lines && <Breakdown lines={quote.lines} coverages={quote.selection.coverages} total={quote.total_premium_kurus} />}

      {error && <div className="notice error" role="alert">{error}</div>}
      {issued && (
        <div className="notice success" role="status">
          Poliçe düzenlendi: <strong>{issued.policy_no}</strong> ({issued.start_date} → {issued.end_date},{' '}
          {formatTL(issued.annual_premium_kurus)})
        </div>
      )}
      {canConvert && (
        <button type="button" onClick={convert} disabled={busy}>
          {busy ? 'Çevriliyor…' : 'Poliçeye çevir'}
        </button>
      )}
      {canReview && (
        <div className="review">
          <label>
            Not (red için zorunlu)
            <textarea value={note} onChange={(e) => setNote(e.target.value)} maxLength={1000} rows={2} />
          </label>
          <div className="actions">
            <button type="button" onClick={() => review('APPROVE')} disabled={busy}>Onayla</button>
            <button type="button" className="danger" onClick={() => review('REJECT')} disabled={busy}>Reddet</button>
          </div>
        </div>
      )}
    </section>
  )
}
