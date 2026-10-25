import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { ProductionReport, ProductionRow } from '../api/types'
import { Empty, ErrorBox, Loading } from '../components/States'
import { errorText, formatTL } from '../format'

function ReportRow({ row }: { row: ProductionRow }) {
  return <tr><td data-label="Grup">{row.label}</td><td data-label="Poliçe adedi">{row.policy_count}</td><td data-label="Yazılan net prim">{formatTL(row.written_premium_kurus)}</td><td data-label="Tazminat">{formatTL(row.incurred_claims_kurus)}</td><td data-label="Hasar / prim">{row.loss_ratio_bp === null ? '—' : `%${(row.loss_ratio_bp / 100).toLocaleString('tr-TR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`}</td></tr>
}

export function ReportsPage() {
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [group, setGroup] = useState('month')
  const [query, setQuery] = useState('group_by=month')
  const [report, setReport] = useState<ProductionReport | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let cancelled = false
    api<ProductionReport>(`/api/v1/reports/production?${query}`).then((r) => { if (!cancelled) { setReport(r); setError(null) } }).catch((e: unknown) => !cancelled && setError(errorText(e)))
    return () => { cancelled = true }
  }, [query, attempt])
  return <section>
    <form className="form-grid" onSubmit={(e) => { e.preventDefault(); setReport(null); setError(null); setQuery(new URLSearchParams({ from, to, group_by: group }).toString()); setAttempt((a) => a + 1) }}>
      <label>Başlangıç (dahil)<input type="date" value={from} onChange={(e) => setFrom(e.target.value)} /></label>
      <label>Bitiş (hariç)<input type="date" value={to} onChange={(e) => setTo(e.target.value)} /></label>
      <label>Gruplama<select value={group} onChange={(e) => setGroup(e.target.value)}><option value="month">Ay</option><option value="product">Ürün</option><option value="agent">Acente</option></select></label>
      <button>Raporu getir</button>
    </form>
    {error ? <ErrorBox message={error} onRetry={() => setAttempt((a) => a + 1)} /> : !report ? <Loading text="Rapor yükleniyor…" /> : <>
      {report.items.length === 0 && <Empty text="Bu dönemde kayıt yok." />}
      <div className="table-wrap report-table"><table><thead><tr><th>Grup</th><th>Poliçe adedi</th><th>Yazılan net prim</th><th>Tahakkuk eden tazminat</th><th>Hasar / prim</th></tr></thead><tbody>{report.items.map((r) => <ReportRow key={r.key} row={r} />)}</tbody><tfoot><ReportRow row={report.total} /></tfoot></table></div>
    </>}
    <p>Yazılan prim bazlı; kazanılmış prim ve muallak yok (kurgusal model)</p>
  </section>
}
