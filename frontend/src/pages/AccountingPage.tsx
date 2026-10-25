import { useEffect, useState, type FormEvent } from 'react'
import { api } from '../api/client'
import type { BillingRun, JournalEntry, TrialBalance } from '../api/types'
import { Empty, ErrorBox, Loading } from '../components/States'
import { errorText, formatTL, todayISO } from '../format'
import { usePaged } from '../hooks/usePaged'

export function AccountingPage() {
  const [trial, setTrial] = useState<TrialBalance | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)
  const [asOf, setAsOf] = useState(todayISO())
  const [run, setRun] = useState<BillingRun | null>(null)
  const [busy, setBusy] = useState(false)
  const journal = usePaged<JournalEntry>('/api/v1/journal-entries?limit=20')

  useEffect(() => {
    let cancelled = false
    api<TrialBalance>('/api/v1/ledger/trial-balance')
      .then((b) => { if (!cancelled) setTrial(b) })
      .catch((e: unknown) => { if (!cancelled) setError(errorText(e)) })
    return () => { cancelled = true }
  }, [attempt])

  function reload() {
    setTrial(null)
    setError(null)
    setAttempt((a) => a + 1)
    journal.reload()
  }

  async function runBilling(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      let after = 0
      const total: BillingRun = { as_of: asOf, accrual_count: 0, interest_kurus: 0, cancelled: [], skipped: [], failed: [], done: false, next_after_policy_id: 0 }
      do {
        const batch = await api<BillingRun>('/api/v1/billing/run', { method: 'POST', body: JSON.stringify({ as_of: asOf, after_policy_id: after }) })
        total.accrual_count += batch.accrual_count
        total.interest_kurus += batch.interest_kurus
        total.cancelled.push(...batch.cancelled)
        total.skipped.push(...batch.skipped)
        total.failed.push(...batch.failed)
        total.done = batch.done
        total.next_after_policy_id = batch.next_after_policy_id
        setRun({ ...total })
        if (!batch.done && batch.next_after_policy_id <= after) throw new Error('Gün sonu imleci ilerlemedi.')
        after = batch.next_after_policy_id
      } while (!total.done)
      reload()
    } catch (e) {
      setError(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  return <section>
    <p>Tahsilat ve iade için Poliçeler menüsünden poliçeyi açın.</p>
    <form className="inline-form" onSubmit={(e) => void runBilling(e)}>
      <label>Gün sonu tarihi<input type="date" required max={todayISO()} value={asOf} onChange={(e) => setAsOf(e.target.value)} disabled={busy} /></label>
      <button disabled={busy}>{busy ? 'Çalışıyor…' : 'Gün sonu çalıştır'}</button>
    </form>
    {run && <div className={`notice ${run.failed.length ? 'error' : 'success'}`} role="status">{run.done ? 'Tamamlandı' : busy ? `İşleniyor (#${run.next_after_policy_id})` : `Yarım kaldı (#${run.next_after_policy_id})`} · {run.as_of}: {run.accrual_count} faiz kaydı, {formatTL(run.interest_kurus)}; iptal: {run.cancelled.join(', ') || 'yok'}; atlanan: {run.skipped.map((p) => `#${p.policy_id} ${p.reason}`).join(', ') || 'yok'}; hata: {run.failed.map((p) => `#${p.policy_id} ${p.code}`).join(', ') || 'yok'}</div>}
    {run && (run.failed.length > 0 || run.skipped.length > 0) && <p>Atlanan veya başarısız poliçeler yeni bir gün sonu koşusunda yeniden denenir.</p>}
    {error && <ErrorBox message={error} onRetry={reload} />}
    {!trial && !error && <Loading text="Mizan yükleniyor…" />}
    {trial && <section className="panel">
      <h3>Mizan ve mutabakat</h3>
      <div className={`notice ${trial.balanced && trial.receivable_reconciliation.matches ? 'success' : 'error'}`}>
        Defter {trial.balanced ? 'dengede' : 'dengesiz'} · Mutabakat {trial.receivable_reconciliation.matches ? 'sağlandı' : 'sağlanamadı'} · Satırsız: {trial.empty_entries}, dengesiz: {trial.unbalanced_entries}
      </div>
      <div className="table-wrap"><table><thead><tr><th>Hesap</th><th>Borç</th><th>Alacak</th><th>Bakiye</th></tr></thead><tbody>{trial.accounts.map((a) => <tr key={a.code}>
        <td data-label="Hesap">{a.code} {a.name}</td><td data-label="Borç">{formatTL(a.debit_kurus)}</td><td data-label="Alacak">{formatTL(a.credit_kurus)}</td><td data-label="Bakiye">{formatTL(a.balance_kurus)}</td>
      </tr>)}</tbody></table></div>
      <p>Toplam borç {formatTL(trial.total_debit_kurus)} = toplam alacak {formatTL(trial.total_credit_kurus)}</p>
      <p>1200: {formatTL(trial.receivable_reconciliation.ledger_1200)} · Açık anapara {formatTL(trial.receivable_reconciliation.open_installments)} + faiz {formatTL(trial.receivable_reconciliation.open_interest)}</p>
    </section>}
    <button className="secondary" onClick={reload}>Mizanı ve defteri yenile</button>
    <h3>Yevmiye kayıtları</h3>
    {journal.state.kind === 'loading' && <Loading />}
    {journal.state.kind === 'error' && <ErrorBox message={journal.state.message} onRetry={journal.reload} />}
    {journal.state.kind === 'ready' && (journal.state.items.length === 0 ? <Empty text="Yevmiye kaydı yok." /> : <>
      {journal.state.items.map((e) => <details key={e.id} className="endorsement"><summary>#{e.id} · {e.entry_date} · {e.description}</summary>
        <p>{e.ref_type} / #{e.ref_id}</p>
        <div className="table-wrap"><table><thead><tr><th>Hesap</th><th>Borç</th><th>Alacak</th></tr></thead><tbody>{e.lines.map((l) => <tr key={l.id}>
          <td data-label="Hesap">{l.account_code}</td><td data-label="Borç">{formatTL(l.debit_kurus)}</td><td data-label="Alacak">{formatTL(l.credit_kurus)}</td>
        </tr>)}</tbody></table></div>
      </details>)}
      {journal.state.next && <button className="secondary" disabled={journal.state.loadingMore} onClick={journal.loadMore}>{journal.state.loadingMore ? 'Yükleniyor…' : 'Daha fazla kayıt'}</button>}
    </>)}
  </section>
}
