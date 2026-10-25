import { useEffect, useState, type FormEvent } from 'react'
import { api } from '../api/client'
import type { ClaimItem, CoverageOnDate } from '../api/types'
import { errorText, newIdempotencyKey, todayISO } from '../format'
import { ErrorBox, Loading } from './States'

export function ReportClaimForm({ policyID, onDone }: { policyID: number; onDone: () => void }) {
  const [loss, setLoss] = useState(todayISO())
  const [coverage, setCoverage] = useState('')
  const [version, setVersion] = useState<CoverageOnDate | null>(null)
  const [coverageError, setCoverageError] = useState<string | null>(null)
  const [description, setDescription] = useState('')
  const [amount, setAmount] = useState('0')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [key, setKey] = useState(newIdempotencyKey)
  const [created, setCreated] = useState<ClaimItem | null>(null)
  useEffect(() => {
    let cancelled = false
    api<CoverageOnDate>(`/api/v1/policies/${policyID}/coverage?date=${encodeURIComponent(loss)}`)
      .then((v) => { if (!cancelled) { setVersion(v); setCoverage(v.version.coverages[0]?.code ?? ''); setCoverageError(null) } })
      .catch((e: unknown) => { if (!cancelled) { setVersion(null); setCoverageError(errorText(e)) } })
    return () => { cancelled = true }
  }, [policyID, loss])
  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!version || version.date !== loss) return
    setBusy(true); setError(null)
    try {
      const c = await api<ClaimItem>('/api/v1/claims', { method: 'POST', headers: { 'Idempotency-Key': key }, body: JSON.stringify({ policy_id: policyID, coverage_code: coverage, loss_date: loss, description, claimed_amount_kurus: Number(amount) }) })
      setCreated(c)
    } catch (e) { setError(errorText(e)) } finally { setBusy(false) }
  }
  if (created) return <div className="notice" role="status">{created.claim_no} bildirildi. <button type="button" onClick={onDone}>Tamam</button></div>
  return (
    <form className="panel" onSubmit={(e) => void submit(e)} onChange={() => setKey(newIdempotencyKey())}>
      <h4>Hasar bildir</h4>
      <div className="form-grid">
        <label>Hasar tarihi<input type="date" required max={todayISO()} value={loss} onChange={(e) => setLoss(e.target.value)} disabled={busy} /></label>
        <label>Teminat<select required value={coverage} onChange={(e) => setCoverage(e.target.value)} disabled={busy || !version || version.date !== loss}>
          {version?.version.coverages.map((c) => <option key={c.code} value={c.code}>{c.name ?? c.code}</option>)}
        </select></label>
        <label>Talep tutarı (kuruş)<input type="number" min="0" max={Number.MAX_SAFE_INTEGER} step="1" required value={amount} onChange={(e) => setAmount(e.target.value)} disabled={busy} /></label>
        <label>Açıklama<textarea required maxLength={4000} value={description} onChange={(e) => setDescription(e.target.value)} disabled={busy} /></label>
      </div>
      {coverageError && <ErrorBox message={coverageError} />}
      {!coverageError && (!version || version.date !== loss) && <Loading text="Hasar tarihindeki teminat aranıyor…" />}
      {version && version.date === loss && <p>Hasar v{version.version.version_no} sürümüne bağlanacak.</p>}
      {error && <ErrorBox message={error} />}
      <button disabled={busy || !version || version.date !== loss}>{busy ? 'Kaydediliyor…' : 'Hasarı bildir'}</button>
    </form>
  )
}

export function AppraisalForm({ claimID, onDone }: { claimID: number; onDone: () => void }) {
  const [amount, setAmount] = useState('0')
  const [totalLoss, setTotalLoss] = useState(false)
  const [notes, setNotes] = useState('')
  const [inspected, setInspected] = useState(() => {
    const d = new Date()
    return new Date(d.getTime() - d.getTimezoneOffset() * 60_000).toISOString().slice(0, 16)
  })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  async function submit(e: FormEvent) {
    e.preventDefault(); setBusy(true); setError(null)
    try {
      await api(`/api/v1/claims/${claimID}/appraisals`, { method: 'POST', body: JSON.stringify({ damage_amount_kurus: Number(amount), is_total_loss: totalLoss, notes, inspected_at: new Date(inspected).toISOString() }) })
      onDone()
    } catch (e) { setError(errorText(e)) } finally { setBusy(false) }
  }
  return <form className="panel" onSubmit={(e) => void submit(e)}>
    <h4>Ekspertiz ekle</h4>
    <div className="form-grid">
      <label>Brüt hasar (kuruş)<input type="number" required min="0" max={Number.MAX_SAFE_INTEGER} step="1" value={amount} onChange={(e) => setAmount(e.target.value)} disabled={busy} /></label>
      <label>İnceleme zamanı<input type="datetime-local" required value={inspected} onChange={(e) => setInspected(e.target.value)} disabled={busy} /></label>
      <label>Pert durumu<select value={totalLoss ? 'yes' : 'no'} onChange={(e) => setTotalLoss(e.target.value === 'yes')} disabled={busy}><option value="no">Kısmi hasar</option><option value="yes">Tam hasar</option></select></label>
      <label>Not<textarea maxLength={4000} value={notes} onChange={(e) => setNotes(e.target.value)} disabled={busy} /></label>
    </div>
    {error && <ErrorBox message={error} />}
    <button disabled={busy}>{busy ? 'Kaydediliyor…' : 'Ekspertizi kaydet'}</button>
  </form>
}
