// API istemcisi: access token bellekte tutulur; refresh token httpOnly cookie'dedir ve
// JavaScript tarafından okunamaz. 401 gelirse bir kez yenileyip isteği tekrarlar.

export type Role =
  | 'CUSTOMER'
  | 'AGENT'
  | 'UNDERWRITER'
  | 'APPRAISER'
  | 'CLAIMS_ADJUSTER'
  | 'ACCOUNTING'

export interface User {
  id: number
  email: string
  full_name: string
  role: Role
}

export interface Session {
  access_token: string
  token_type: 'Bearer'
  expires_in: number
  user: User
}

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly details?: Record<string, string>

  constructor(status: number, code: string, message: string, details?: Record<string, string>) {
    super(message)
    this.status = status
    this.code = code
    this.details = details
  }
}

let accessToken: string | null = null
let refreshInFlight: Promise<Session> | null = null

export function setAccessToken(token: string | null) {
  accessToken = token
}

async function parse<T>(res: Response): Promise<T> {
  const text = await res.text()
  const body = text ? JSON.parse(text) : null
  if (!res.ok) {
    const e = body?.error
    throw new ApiError(res.status, e?.code ?? 'HTTP_ERROR', e?.message ?? `HTTP ${res.status}`, e?.details)
  }
  return body as T
}

async function send(path: string, init: RequestInit): Promise<Response> {
  const headers = new Headers(init.headers)
  if (init.body) headers.set('Content-Type', 'application/json')
  if (accessToken) headers.set('Authorization', `Bearer ${accessToken}`)
  try {
    return await fetch(path, { ...init, headers, credentials: 'same-origin' })
  } catch {
    throw new ApiError(0, 'NETWORK_ERROR', 'Sunucuya ulaşılamadı')
  }
}

// refreshSession eşzamanlı çağrıları tek isteğe indirir: sunucu kullanılmış refresh
// token'ı çalınma sayıp oturumu iptal ettiği için paralel yenileme yapılmamalı.
export function refreshSession(): Promise<Session> {
  if (!refreshInFlight) {
    refreshInFlight = send('/api/v1/auth/refresh', { method: 'POST' })
      .then((res) => parse<Session>(res))
      .then((s) => {
        setAccessToken(s.access_token)
        return s
      })
      .finally(() => {
        refreshInFlight = null
      })
  }
  return refreshInFlight
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  let res = await send(path, init)
  if (res.status === 401 && accessToken) {
    try {
      await refreshSession()
    } catch {
      setAccessToken(null)
      return parse<T>(res)
    }
    res = await send(path, init)
  }
  return parse<T>(res)
}

export async function login(email: string, password: string): Promise<Session> {
  const res = await send('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  })
  const s = await parse<Session>(res)
  setAccessToken(s.access_token)
  return s
}

export async function logout(): Promise<void> {
  try {
    await api<void>('/api/v1/auth/logout', { method: 'POST' })
  } finally {
    setAccessToken(null)
  }
}
