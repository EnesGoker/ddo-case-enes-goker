import { useEffect, useState, type FormEvent } from 'react'
import { api } from '../api/client'
import type { Calculation, Customer, Page, Product, Quote, QuoteRequest, Tariff } from '../api/types'
import { Breakdown } from '../components/Breakdown'
import { QuoteDetail } from '../components/QuoteDetail'
import { ErrorBox, Loading } from '../components/States'
import { errorText, formatTL, reasonLabels } from '../format'

const products: { value: Product; label: string }[] = [
  { value: 'KASKO', label: 'Kasko' },
  { value: 'TRAFIK', label: 'Zorunlu trafik' },
  { value: 'KONUT', label: 'Konut' },
]

function tomorrow(): string {
  const d = new Date()
  d.setDate(d.getDate() + 1)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

type Loadable<T> = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; value: T }

function factorKeys(t: Tariff, type: string): string[] {
  return t.factors.filter((f) => f.type === type && f.key !== undefined).map((f) => f.key!).sort()
}

export function QuoteFormPage() {
  const [customers, setCustomers] = useState<Loadable<Customer[]>>({ kind: 'loading' })
  const [tariff, setTariff] = useState<Loadable<Tariff>>({ kind: 'loading' })
  const [form, setForm] = useState({
    customerId: 0, product: 'KASKO' as Product, termStart: tomorrow(), ncd: '0', vehicleGroup: 'B', plate: '',
  })
  const [optional, setOptional] = useState<Set<string>>(new Set())
  const [preview, setPreview] = useState<Calculation | null>(null)
  const [saved, setSaved] = useState<Quote | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    api<Page<Customer>>('/api/v1/customers?limit=100')
      .then((p) => !cancelled && setCustomers({ kind: 'ready', value: p.items }))
      .catch((e: unknown) => !cancelled && setCustomers({ kind: 'error', message: errorText(e) }))
    return () => {
      cancelled = true
    }
  }, [])

  // Tarife, seçilen ürün ve poliçe başlangıç tarihine göre (sunucudaki kural ile aynı)
  useEffect(() => {
    let cancelled = false
    api<Tariff>(`/api/v1/tariffs/active?product=${form.product}&date=${form.termStart}`)
      .then((t) => !cancelled && setTariff({ kind: 'ready', value: t }))
      .catch((e: unknown) => !cancelled && setTariff({ kind: 'error', message: errorText(e) }))
    return () => {
      cancelled = true
    }
  }, [form.product, form.termStart])

  function update(patch: Partial<typeof form>) {
    setForm((f) => ({ ...f, ...patch }))
    setPreview(null)
    setError(null)
    if (patch.product || patch.termStart) setTariff({ kind: 'loading' })
    if (patch.product) setOptional(new Set())
  }

  function toggle(code: string) {
    setOptional((s) => {
      const n = new Set(s)
      if (n.has(code)) n.delete(code)
      else n.add(code)
      return n
    })
    setPreview(null)
  }

  if (customers.kind === 'loading') return <Loading text="Müşteriler yükleniyor…" />
  if (customers.kind === 'error') return <ErrorBox message={customers.message} />
  if (customers.value.length === 0) return <div className="notice">Teklif için önce bir müşteri kaydı gerekli.</div>
  if (saved) {
    return (
      <>
        <QuoteDetail quote={saved} role="AGENT" onChanged={setSaved} />
        <button type="button" className="secondary" onClick={() => { setSaved(null); setPreview(null) }}>
          Yeni teklif
        </button>
      </>
    )
  }

  const t = tariff.kind === 'ready' ? tariff.value : null
  const isVehicle = form.product !== 'KONUT'
  const customerId = form.customerId || customers.value[0].id

  function request(): QuoteRequest {
    const required = t ? t.coverages.filter((c) => c.required).map((c) => c.code) : []
    return {
      customer_id: customerId,
      product: form.product,
      term_start: form.termStart,
      ncd_level: Number(form.ncd),
      vehicle_group: isVehicle ? form.vehicleGroup : undefined,
      plate: isVehicle ? form.plate : undefined,
      coverages: [...required, ...optional],
    }
  }

  async function send<T>(path: string): Promise<T | null> {
    setBusy(true)
    setError(null)
    try {
      return await api<T>(path, { method: 'POST', body: JSON.stringify(request()) })
    } catch (e) {
      setError(errorText(e))
      return null
    } finally {
      setBusy(false)
    }
  }

  async function onPreview(e: FormEvent) {
    e.preventDefault()
    setPreview(await send<Calculation>('/api/v1/quotes/preview'))
  }

  async function onSave() {
    const q = await send<Quote>('/api/v1/quotes')
    if (q) setSaved(q)
  }

  return (
    <section>
      <form className="form-grid" onSubmit={onPreview}>
        <label>
          Müşteri
          <select value={customerId} onChange={(e) => update({ customerId: Number(e.target.value) })}>
            {customers.value.map((c) => (
              <option key={c.id} value={c.id}>{c.full_name} ({c.national_id_masked}, bölge {c.region_code})</option>
            ))}
          </select>
        </label>
        <label>
          Ürün
          <select value={form.product} onChange={(e) => update({ product: e.target.value as Product })}>
            {products.map((p) => <option key={p.value} value={p.value}>{p.label}</option>)}
          </select>
        </label>
        <label>
          Başlangıç tarihi
          <input type="date" value={form.termStart} onChange={(e) => update({ termStart: e.target.value })} required />
        </label>
        {t && (
          <label>
            Hasarsızlık kademesi
            <select value={form.ncd} onChange={(e) => update({ ncd: e.target.value })}>
              {factorKeys(t, 'NCD').map((k) => <option key={k} value={k}>{k}</option>)}
            </select>
          </label>
        )}
        {t && isVehicle && (
          <>
            <label>
              Araç grubu
              <select value={form.vehicleGroup} onChange={(e) => update({ vehicleGroup: e.target.value })}>
                {factorKeys(t, 'VEHICLE_GROUP').map((k) => <option key={k} value={k}>{k}</option>)}
              </select>
            </label>
            <label>
              Plaka
              <input value={form.plate} placeholder="34ABC123" onChange={(e) => update({ plate: e.target.value })} />
            </label>
          </>
        )}

        {tariff.kind === 'loading' && <Loading text="Tarife yükleniyor…" />}
        {tariff.kind === 'error' && <ErrorBox message={tariff.message} />}
        {t && (
          <fieldset className="coverages">
            <legend>Teminatlar — tarife {t.product} v{t.version_no} ({t.valid_from}{t.valid_to ? ` → ${t.valid_to}` : ''})</legend>
            {t.coverages.map((c) => (
              <label key={c.code} className="check">
                <input type="checkbox" checked={c.required || optional.has(c.code)} disabled={c.required}
                  onChange={() => toggle(c.code)} />
                {c.name} — limit {formatTL(c.limit_kurus)}{c.required ? ' (zorunlu)' : ''}
              </label>
            ))}
          </fieldset>
        )}

        {error && <div className="notice error" role="alert">{error}</div>}
        <div className="actions">
          <button type="submit" disabled={busy || !t}>{busy ? 'Hesaplanıyor…' : 'Primi hesapla'}</button>
          {preview && <button type="button" onClick={onSave} disabled={busy}>Teklifi kaydet</button>}
        </div>
      </form>

      {preview && (
        <section className="panel">
          <h3>Önizleme — {formatTL(preview.total_premium_kurus)}</h3>
          {preview.review_required && (
            <div className="notice warn">
              Underwriter onayı gerekecek: {preview.review_reasons.map((r) => reasonLabels[r] ?? r).join(', ')}
            </div>
          )}
          <Breakdown lines={preview.lines} coverages={preview.selection.coverages} total={preview.total_premium_kurus} />
          <p className="muted">Dönem {preview.term_start} → {preview.term_end}. Vergi/harç dahil değildir (örnek tarife).</p>
        </section>
      )}
    </section>
  )
}
