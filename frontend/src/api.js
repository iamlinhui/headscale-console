// Production HTML is served at / or /admin/; preserve the mounted path for APIs.
const apiBase = import.meta.env.DEV ? '/api' : new URL('api', new URL('./', window.location.href)).pathname.replace(/\/$/, '')
export class ApiError extends Error {
  constructor(message, status) { super(message); this.status = status }
}
export async function api(path, { method = 'GET', body, csrf } = {}) {
  let response
  try {
    response = await fetch(`${apiBase}${path}`, {
      redirect: 'error',
      method, credentials: 'same-origin', cache: 'no-store',
      headers: { ...(body ? { 'Content-Type': 'application/json' } : {}), ...(csrf ? { 'X-CSRF-Token': csrf } : {}) },
      ...(body ? { body: JSON.stringify(body) } : {})
    })
  } catch { throw new ApiError('无法连接控制台后端，请检查服务是否启动。', 0) }
  const data = await response.json().catch(() => ({}))
  if (!response.ok) throw new ApiError(data.error || data.message || '请求失败', response.status)
  return data
}
