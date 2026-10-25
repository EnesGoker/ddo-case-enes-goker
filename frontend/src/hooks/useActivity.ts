import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { Activity } from '../api/types'
import { errorText } from '../format'

export function useActivity() {
  const [activity, setActivity] = useState<Activity | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)
  const [checkedAt, setCheckedAt] = useState(0)
  useEffect(() => {
    let cancelled = false
    let timer = 0
    const controller = new AbortController()
    async function poll() {
      try {
        const data = await api<Activity>('/api/v1/system/activity', { signal: controller.signal })
        if (!cancelled) { setActivity(data); setError(null); setCheckedAt(Date.now()) }
      } catch (e) {
        if (!cancelled) setError(errorText(e))
      } finally {
        if (!cancelled) timer = window.setTimeout(() => void poll(), 15000)
      }
    }
    void poll()
    return () => { cancelled = true; controller.abort(); window.clearTimeout(timer) }
  }, [attempt])
  return { activity, error, checkedAt, reload: () => setAttempt((a) => a + 1) }
}
