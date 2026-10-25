import type { Customer } from '../api/types'
import { Empty, ErrorBox, Loading } from '../components/States'
import { usePaged } from '../hooks/usePaged'

export function CustomersPage() {
  const { state, reload, loadMore } = usePaged<Customer>('/api/v1/customers?limit=20')

  if (state.kind === 'loading') return <Loading text="Müşteriler yükleniyor…" />
  if (state.kind === 'error') return <ErrorBox message={state.message} onRetry={reload} />
  if (state.items.length === 0) return <Empty text="Henüz müşteri kaydı yok." />

  return (
    <section>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>No</th>
              <th>Ad Soyad</th>
              <th>TCKN</th>
              <th>Doğum tarihi</th>
              <th>Bölge</th>
            </tr>
          </thead>
          <tbody>
            {state.items.map((c) => (
              <tr key={c.id}>
                <td data-label="No">{c.id}</td>
                <td data-label="Ad Soyad">{c.full_name}</td>
                <td data-label="TCKN">{c.national_id_masked}</td>
                <td data-label="Doğum tarihi">{c.birth_date}</td>
                <td data-label="Bölge">{c.region_code}</td>
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
    </section>
  )
}
