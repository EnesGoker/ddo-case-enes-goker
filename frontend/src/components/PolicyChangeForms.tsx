import { useEffect, useState, type FormEvent } from 'react'
import { api } from '../api/client'
import type { Role } from '../api/client'
import type { CancelPreview, PolicyDetail, Tariff, VehicleChangePreview } from '../api/types'
import { addDays, cancelReasonLabels, errorText, formatTL, maxISO, newIdempotencyKey, todayISO } from '../format'
import { Breakdown } from './Breakdown'
import { CalcSteps } from './CalcSteps'

function openVersion(p: PolicyDetail) {
  return p.versions[p.versions.length - 1]
}

// Önizleme sunucudan gelir; onay aynı istek gövdesi + önizlemede üretilen Idempotency-Key ile gönderilir.
// base_version_no ile önizlemeden sonra poliçe değiştiyse sunucu 409 STALE_VERSION döner.
export function VehicleChangeForm({ policy, onDone }: { policy: PolicyDetail; onDone: () => void }) {
  const open = openVersion(policy)
  const minDate = addDays(open.effective_from, 1)
  const [date, setDate] = useState(maxISO(addDays(todayISO(), 1), minDate))
  const [group, setGroup] = useState(open.risk.vehicle_group ?? '')
  const [plate, setPlate] = useState(open.vehicle_plate ?? '')
  const [limitsTL, setLimitsTL] = useState<Record<string, string>>({})
  const [groups, setGroups] = useState<string[]>([])
  const [preview, setPreview] = useState<{ data: VehicleChangePreview; key: string; body: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    api<Tariff>(`/api/v1/tariffs/active?product=${policy.product}&date=${policy.start_date}`)
      .then((t) => {
        if (!cancelled) setGroups(t.factors.filter((f) => f.type === 'VEHICLE_GROUP' && f.key).map((f) => f.key!).sort())
      })
      .catch(() => !cancelled && setGroups([]))
    return () => {
      cancelled = true
    }
  }, [policy.product, policy.start_date])

  function body(): string {
    const coverage_limits: Record<string, number> = {}
    for (const [code, tl] of Object.entries(limitsTL)) {
      const n = Number.parseInt(tl, 10)
      if (Number.isFinite(n) && n > 0) coverage_limits[code] = n * 100
    }
    return JSON.stringify({ effective_date: date, vehicle_group: group, plate, coverage_limits, base_version_no: open.version_no })
  }

  function changed() {
    setPreview(null)
    setError(null)
  }

  async function onPreview(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    const b = body()
    try {
      const data = await api<VehicleChangePreview>(`/api/v1/policies/${policy.id}/endorsements/preview`, { method: 'POST', body: b })
      setPreview({ data, key: newIdempotencyKey(), body: b })
    } catch (err) {
      setError(errorText(err))
    } finally {
      setBusy(false)
    }
  }

  async function onConfirm() {
    if (!preview) return
    setBusy(true)
    setError(null)
    try {
      await api(`/api/v1/policies/${policy.id}/endorsements`, {
        method: 'POST', body: preview.body, headers: { 'Idempotency-Key': preview.key },
      })
      onDone()
    } catch (err) {
      setError(errorText(err))
      setBusy(false)
    }
  }

  const groupOptions = groups.length > 0 ? groups : [group]
  return (
    <section className="panel">
      <h3>Araç değişikliği zeyli</h3>
      <form className="form-grid" onSubmit={onPreview}>
        <label>
          Zeyil tarihi
          <input type="date" value={date} min={minDate} max={addDays(policy.end_date, -1)} required
            onChange={(e) => { setDate(e.target.value); changed() }} />
        </label>
        <label>
          Araç grubu
          <select value={group} onChange={(e) => { setGroup(e.target.value); changed() }}>
            {groupOptions.map((g) => <option key={g} value={g}>{g}</option>)}
          </select>
        </label>
        <label>
          Plaka
          <input value={plate} onChange={(e) => { setPlate(e.target.value); changed() }} />
        </label>
        <fieldset className="coverages">
          <legend>Teminat limitleri (TL, isteğe bağlı; boş = değişmez)</legend>
          {open.coverages.map((c) => (
            <label key={c.code}>
              {c.name ?? c.code} — şu an {formatTL(c.limit_kurus)}
              <input type="number" min={1} step={1} inputMode="numeric" value={limitsTL[c.code] ?? ''}
                placeholder="yeni limit (TL)"
                onChange={(e) => { setLimitsTL({ ...limitsTL, [c.code]: e.target.value }); changed() }} />
            </label>
          ))}
        </fieldset>
        {error && <div className="notice error" role="alert">{error}</div>}
        <div className="actions">
          <button type="submit" disabled={busy}>{busy && !preview ? 'Hesaplanıyor…' : 'Önizle'}</button>
          {preview && (
            <button type="button" onClick={onConfirm} disabled={busy}>{busy ? 'Kaydediliyor…' : 'Zeyli onayla'}</button>
          )}
        </div>
      </form>
      {preview && (
        <div>
          <div className={`notice ${preview.data.premium_delta_kurus < 0 ? 'success' : 'warn'}`}>
            Yıllık prim {formatTL(preview.data.old_annual_premium_kurus)} → {formatTL(preview.data.new_annual_premium_kurus)} ·{' '}
            {preview.data.premium_delta_kurus >= 0 ? 'Ek prim' : 'İade'}: <strong>{formatTL(Math.abs(preview.data.premium_delta_kurus))}</strong>
            {' '}(sürüm {preview.data.base_version_no} → {preview.data.new_version_no})
          </div>
          <CalcSteps calc={preview.data.calc_detail} />
          <Breakdown lines={preview.data.lines} coverages={preview.data.coverages} total={preview.data.new_annual_premium_kurus} />
        </div>
      )}
    </section>
  )
}

export function CancelForm({ policy, role, onDone }: { policy: PolicyDetail; role: Role; onDone: () => void }) {
  const open = openVersion(policy)
  const reasons = role === 'UNDERWRITER' ? ['CUSTOMER_REQUEST', 'UNDERWRITER', 'NON_PAYMENT', 'OTHER'] : ['CUSTOMER_REQUEST']
  const [date, setDate] = useState(maxISO(addDays(todayISO(), 1), open.effective_from))
  const [reason, setReason] = useState(reasons[0])
  const [note, setNote] = useState('')
  const [preview, setPreview] = useState<{ data: CancelPreview; key: string; body: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function changed() {
    setPreview(null)
    setError(null)
  }

  async function onPreview(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    const b = JSON.stringify({ effective_date: date, reason, note, base_version_no: open.version_no })
    try {
      const data = await api<CancelPreview>(`/api/v1/policies/${policy.id}/cancel/preview`, { method: 'POST', body: b })
      setPreview({ data, key: newIdempotencyKey(), body: b })
    } catch (err) {
      setError(errorText(err))
    } finally {
      setBusy(false)
    }
  }

  async function onConfirm() {
    if (!preview) return
    setBusy(true)
    setError(null)
    try {
      await api(`/api/v1/policies/${policy.id}/cancel`, {
        method: 'POST', body: preview.body, headers: { 'Idempotency-Key': preview.key },
      })
      onDone()
    } catch (err) {
      setError(errorText(err))
      setBusy(false)
    }
  }

  return (
    <section className="panel">
      <h3>Poliçe iptali</h3>
      <form className="form-grid" onSubmit={onPreview}>
        <label>
          İptal tarihi
          <input type="date" value={date} min={open.effective_from} max={addDays(policy.end_date, -1)} required
            onChange={(e) => { setDate(e.target.value); changed() }} />
        </label>
        <label>
          Gerekçe
          <select value={reason} onChange={(e) => { setReason(e.target.value); changed() }}>
            {reasons.map((r) => <option key={r} value={r}>{cancelReasonLabels[r]}</option>)}
          </select>
        </label>
        <label>
          Not
          <input value={note} maxLength={1000} onChange={(e) => { setNote(e.target.value); changed() }} />
        </label>
        {error && <div className="notice error" role="alert">{error}</div>}
        <div className="actions">
          <button type="submit" disabled={busy}>{busy && !preview ? 'Hesaplanıyor…' : 'Önizle'}</button>
          {preview && (
            <button type="button" className="danger" onClick={onConfirm} disabled={busy}>
              {busy ? 'İptal ediliyor…' : 'İptali onayla'}
            </button>
          )}
        </div>
      </form>
      {preview && (
        <div>
          <div className="notice warn">
            Yazılmış prim {formatTL(preview.data.written_premium_kurus)} · İade: <strong>{formatTL(preview.data.refund_kurus)}</strong>
            {!preview.data.closes_version && ' · başlangıçtan iptal: hiçbir gün teminat kalmaz'}
          </div>
          <CalcSteps calc={preview.data.calc_detail} />
        </div>
      )}
    </section>
  )
}
