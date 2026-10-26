import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { AgentPerformance, ProductionRow } from '../api/types'
import { Empty, ErrorBox, Loading } from '../components/States'
import { errorText, formatTL } from '../format'

type SortKey = 'label' | 'policy_count' | 'written_premium_kurus' | 'incurred_claims_kurus' | 'loss_ratio_bp'
const columns: { key: SortKey; label: string }[] = [
  { key: 'label', label: 'Acente' },
  { key: 'policy_count', label: 'Poliçe adedi' },
  { key: 'written_premium_kurus', label: 'Yazılan net prim' },
  { key: 'incurred_claims_kurus', label: 'Tahakkuk eden tazminat' },
  { key: 'loss_ratio_bp', label: 'Hasar / prim (yazılan prime göre)' },
]

function PerformanceRow({ row }: { row: ProductionRow }) {
  return <tr>
    <td>{row.label}</td>
    <td className="num">{row.policy_count.toLocaleString('tr-TR')}</td>
    <td className="num">{formatTL(row.written_premium_kurus)}</td>
    <td className="num">{formatTL(row.incurred_claims_kurus)}</td>
    <td className="num">{row.loss_ratio_bp === null ? '—' : `%${(row.loss_ratio_bp / 100).toLocaleString('tr-TR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`}</td>
  </tr>
}

export function AgentPerformancePage() {
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [query, setQuery] = useState('')
  const [report, setReport] = useState<AgentPerformance | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)
  const [sortKey, setSortKey] = useState<SortKey>('label')
  const [descending, setDescending] = useState(false)

  useEffect(() => {
    let cancelled = false
    api<AgentPerformance>(`/api/v1/reports/agents${query ? `?${query}` : ''}`)
      .then((value) => {
        if (cancelled) return
        setReport(value)
        setError(null)
        if (!query) {
          setFrom(value.from ?? '')
          setTo(value.to ?? '')
        }
      })
      .catch((e: unknown) => { if (!cancelled) setError(errorText(e)) })
    return () => { cancelled = true }
  }, [query, attempt])

  const rows = [...(report?.items ?? [])].sort((a, b) => {
    const left = a[sortKey]
    const right = b[sortKey]
    if (left === null) return right === null ? a.key.localeCompare(b.key) : 1
    if (right === null) return -1
    const comparison = typeof left === 'string' && typeof right === 'string'
      ? left.localeCompare(right, 'tr-TR') : Number(left) - Number(right)
    return comparison === 0 ? a.key.localeCompare(b.key) : descending ? -comparison : comparison
  })
  const hasActivity = rows.some((row) => row.policy_count !== 0 || row.written_premium_kurus !== 0 || row.incurred_claims_kurus !== 0)
  const reload = () => { setReport(null); setError(null); setAttempt((value) => value + 1) }

  return <section className="agent-performance">
    <p>Yazılan prim bazlı acente performansı. Kazanılmış prim hesabı içermez.</p>
    <form className="form-grid" onSubmit={(event) => {
      event.preventDefault()
      const fields = new FormData(event.currentTarget)
      const nextFrom = String(fields.get('from') ?? '')
      const nextTo = String(fields.get('to') ?? '')
      setFrom(nextFrom)
      setTo(nextTo)
      setQuery(new URLSearchParams({ from: nextFrom, to: nextTo }).toString())
      reload()
    }}>
      <label>Başlangıç (dahil)<input name="from" type="date" value={from} onChange={(event) => setFrom(event.target.value)} /></label>
      <label>Bitiş (hariç)<input name="to" type="date" value={to} onChange={(event) => setTo(event.target.value)} /></label>
      <button>Panoyu getir</button>
      <button type="button" className="secondary" onClick={() => { setQuery(''); reload() }}>Son 12 ay</button>
    </form>
    <p className="muted">Varsayılan son 12 takvim ayıdır; iki tarihi boşaltıp getirerek tüm zamanları görebilirsiniz.</p>
    {error ? <ErrorBox message={error} onRetry={reload} /> : !report ? <Loading text="Acente performansı yükleniyor…" /> : <>
      <p className="muted">Dönem: {report.from ?? 'Başlangıç sınırı yok'} → {report.to ? `${report.to} (hariç)` : 'Bitiş sınırı yok'}</p>
      {!rows.length ? <Empty text="Acente kaydı bulunamadı." /> : <>
        {!hasActivity && <Empty text="Bu dönemde hareket yok; acenteler sıfır değerlerle gösteriliyor." />}
        <div className="table-wrap agent-table" tabIndex={0} role="region" aria-label="Acente performans tablosu">
          <table>
            <thead><tr>{columns.map((column) => <th key={column.key} scope="col" aria-sort={sortKey === column.key ? descending ? 'descending' : 'ascending' : 'none'}>
              <button type="button" className="secondary" onClick={() => {
                setDescending(sortKey === column.key ? !descending : false)
                setSortKey(column.key)
              }}>{column.label}{sortKey === column.key ? descending ? ' ↓' : ' ↑' : ''}</button>
            </th>)}</tr></thead>
            <tbody>{rows.map((row) => <PerformanceRow key={row.key} row={row} />)}</tbody>
            {report.total && <tfoot><PerformanceRow row={report.total} /></tfoot>}
          </table>
        </div>
      </>}
    </>}
  </section>
}
