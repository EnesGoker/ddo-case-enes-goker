import { useState } from 'react'
import type { Role } from '../api/client'
import type { Quote } from '../api/types'
import { QuoteDetailLoader } from '../components/QuoteDetail'
import { Empty, ErrorBox, Loading } from '../components/States'
import { formatTL, statusLabels } from '../format'
import { usePaged } from '../hooks/usePaged'
import { StatusBadge } from '../components/StatusBadge'

const filters = ['', 'UW_REVIEW', 'CALCULATED', 'APPROVED', 'REJECTED', 'CONVERTED']

export function QuotesPage({ role }: { role: Role }) {
  const [status, setStatus] = useState(role === 'UNDERWRITER' ? 'UW_REVIEW' : '')
  const [selected, setSelected] = useState<number | null>(null)
  const url = `/api/v1/quotes?limit=20${status ? `&status=${status}` : ''}`
  const { state, reload, loadMore } = usePaged<Quote>(url)

  return (
    <section>
      <div className="filter-bar"><label className="inline">
        Durum{' '}
        <select value={status} onChange={(e) => { setStatus(e.target.value); setSelected(null) }}>
          {filters.map((f) => (
            <option key={f} value={f}>{f ? statusLabels[f] : 'Tümü'}</option>
          ))}
        </select>
      </label></div>

      {state.kind === 'loading' && <Loading text="Teklifler yükleniyor…" />}
      {state.kind === 'error' && <ErrorBox message={state.message} onRetry={reload} />}
      {state.kind === 'ready' && state.items.length === 0 && <Empty text="Bu filtrede teklif yok." />}
      {state.kind === 'ready' && state.items.length > 0 && (
        <>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>No</th>
                  <th>Ürün</th>
                  <th>Sigortalı</th>
                  <th>Başlangıç</th>
                  <th>Durum</th>
                  <th className="num">Prim</th>
                </tr>
              </thead>
              <tbody>
                {state.items.map((q) => (
                  <tr key={q.id} className={`clickable${q.id === selected ? ' selected' : ''}`}
                    onClick={() => setSelected(q.id)} tabIndex={0}
                    onKeyDown={(e) => e.key === 'Enter' && setSelected(q.id)}>
                    <td data-label="No" className="table-id">#{q.id}</td>
                    <td data-label="Ürün">{q.product}</td>
                    <td data-label="Sigortalı">{q.customer_name}</td>
                    <td data-label="Başlangıç">{q.term_start}</td>
                    <td data-label="Durum"><StatusBadge status={q.status} /></td>
                    <td data-label="Prim" className="num">{formatTL(q.total_premium_kurus)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {state.next && (
            <button type="button" className="secondary" onClick={loadMore} disabled={state.loadingMore}>
              {state.loadingMore ? 'Yükleniyor…' : 'Daha fazla'}
            </button>
          )}
        </>
      )}
      {selected !== null && <QuoteDetailLoader id={selected} role={role} onChanged={reload} />}
    </section>
  )
}
