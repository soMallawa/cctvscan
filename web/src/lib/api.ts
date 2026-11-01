export class ApiError extends Error {
  constructor(public status: number, message: string, public field?: string) { super(message); this.name = 'ApiError' }
}
type RequestOptions = { method?: string; body?: unknown; signal?: AbortSignal }
export async function request<T = unknown>(path: string, options: RequestOptions = {}): Promise<T> {
  if (!/^\/(?!\/)/.test(path) || /[\\\r\n]/.test(path)) throw new ApiError(0, 'Only console API paths are allowed')
  let response: Response
  try {
    response = await fetch(`/api${path}`, {method: options.method || 'GET', credentials: 'same-origin', cache: 'no-store',
      headers: {'Accept': 'application/json', ...(options.body !== undefined ? {'Content-Type': 'application/json'} : {})},
      body: options.body !== undefined ? JSON.stringify(options.body) : undefined, signal: options.signal})
  } catch (e) {
    if (e instanceof Error && e.name === 'AbortError') throw e
    throw new ApiError(0, 'Cannot reach the console. Check the connection and try again.')
  }
  if (response.status === 401) throw new ApiError(401, 'Your Access session has ended. Reopen the console to authenticate with Cloudflare.')
  if (response.status === 403) throw new ApiError(403, 'Your Access identity cannot perform this action.')
  if (response.status >= 500) throw new ApiError(response.status, 'The console service is unavailable. Refresh to check the latest saved state before trying again.')
  if (response.status === 204 && response.ok) return undefined as T
  if (!/application\/json|\+json/.test(response.headers.get('Content-Type') || '')) throw new ApiError(response.status || 503, 'The console did not return JSON. Check the Access session and service connection.')
  let data: unknown
  try { data = await response.json() } catch { throw new ApiError(503, 'The console returned an invalid JSON response.') }
  if (!response.ok) {
    const body = data as {error?: unknown; field?: unknown}
    const message = typeof body?.error === 'string' ? body.error.slice(0, 400).replace(/([a-z]+:\/\/)[^\s/@]+@/gi, '$1[redacted]@') : 'The request could not be completed.'
    throw new ApiError(response.status, message, typeof body?.field === 'string' && /^[a-z_.]+$/i.test(body.field) ? body.field : undefined)
  }
  return data as T
}
