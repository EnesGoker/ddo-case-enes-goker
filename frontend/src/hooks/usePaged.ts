import { useCallback, useEffect, useState } from 'react'
import { api } from '../api/client'
import type { Page } from '../api/types'
import { errorText } from '../format'

export type PagedState<T> =
  | { kind: 'loading' }
  | { kind: 'error'; message: string }
  | { kind: 'ready'; items: T[]; next: string | null; loadingMore: boolean }

// usePaged keyset sayfalı bir listeyi yükler; url değişince baştan başlar.
export function usePaged<T>(url: string) {
  const [state, setState] = useState<PagedState<T>>({ kind: 'loading' })
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let cancelled = false
    api<Page<T>>(url)
      .then((p) => !cancelled && setState({ kind: 'ready', items: p.items, next: p.next_cursor, loadingMore: false }))
      .catch((e: unknown) => !cancelled && setState({ kind: 'error', message: errorText(e) }))
    return () => {
      cancelled = true
    }
  }, [url, attempt])

  const reload = useCallback(() => {
    setState({ kind: 'loading' })
    setAttempt((a) => a + 1)
  }, [])

  const loadMore = useCallback(async () => {
    if (state.kind !== 'ready' || !state.next) return
    setState({ ...state, loadingMore: true })
    const sep = url.includes('?') ? '&' : '?'
    try {
      const p = await api<Page<T>>(`${url}${sep}cursor=${encodeURIComponent(state.next)}`)
      setState({ kind: 'ready', items: [...state.items, ...p.items], next: p.next_cursor, loadingMore: false })
    } catch (e) {
      setState({ kind: 'error', message: errorText(e) })
    }
  }, [state, url])

  return { state, reload, loadMore }
}
