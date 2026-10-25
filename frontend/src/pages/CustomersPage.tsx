import { useCallback, useEffect, useState } from 'react'
import { api, ApiError } from '../api/client'
import { Empty, ErrorBox, Loading } from '../components/States'

interface Customer {
  id: number
  full_name: string
  national_id_masked: string
  birth_date: string
  region_code: string
  agent_id: number | null
}

interface Page {
  items: Customer[]
  next_cursor: string | null
}

type State =
  | { kind: 'loading' }
  | { kind: 'error'; message: string }
  | { kind: 'ready'; items: Customer[]; next: string | null; loadingMore: boolean }

const PAGE_SIZE = 20

export function CustomersPage() {
  const [state, setState] = useState<State>({ kind: 'loading' })
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let cancelled = false
    api<Page>(`/api/v1/customers?limit=${PAGE_SIZE}`)
      .then((p) => !cancelled && setState({ kind: 'ready', items: p.items, next: p.next_cursor, loadingMore: false }))
      .catch((e: unknown) => !cancelled && setState({ kind: 'error', message: errorText(e) }))
    return () => {
      cancelled = true
    }
  }, [attempt])

  const retry = useCallback(() => {
    setState({ kind: 'loading' })
    setAttempt((a) => a + 1)
  }, [])

  async function loadMore() {
    if (state.kind !== 'ready' || !state.next) return
    setState({ ...state, loadingMore: true })
    try {
      const p = await api<Page>(`/api/v1/customers?limit=${PAGE_SIZE}&cursor=${encodeURIComponent(state.next)}`)
      setState({ kind: 'ready', items: [...state.items, ...p.items], next: p.next_cursor, loadingMore: false })
    } catch (e) {
      setState({ kind: 'error', message: errorText(e) })
    }
  }

  if (state.kind === 'loading') return <Loading text="Müşteriler yükleniyor…" />
  if (state.kind === 'error') return <ErrorBox message={state.message} onRetry={retry} />
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

function errorText(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 403) return 'Bu listeyi görme yetkiniz yok.'
    return e.message
  }
  return 'Beklenmeyen bir hata oluştu.'
}
