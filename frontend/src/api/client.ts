const BASE_URL = '/api'

const LOGIN_PATH = '/login'

/** Set once we start navigating, so a burst of 401s only redirects once. */
let redirecting = false

/**
 * A session can go invalid at any moment: it expires in a long-open tab, or an
 * admin deletes the account (which cascade-deletes its sessions). Without a
 * central handler every caller has to cope on its own, and the UI dead-ends —
 * even the logout button stops working once POST /logout is the call that 401s.
 *
 * A full page load is the cheapest reliable reset: it clears the Pinia store,
 * the router state and any in-flight requests without this module having to
 * import — and circularly depend on — the auth store or the router. 401s are
 * rare in an admin panel, so the reload costs nothing in practice.
 */
function redirectToLogin() {
  // The login page itself probes GET /api/me through the auth store and gets a
  // 401 whenever nobody is signed in — that is the normal state, not a session
  // loss. Redirecting on it would reload /login forever.
  if (redirecting || window.location.pathname === LOGIN_PATH) {
    return
  }
  redirecting = true
  window.location.href = LOGIN_PATH
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    ...options,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers || {}),
    },
  })

  if (!res.ok) {
    // Only 401 means "your session is gone"; a 404 or 409 from a normal call
    // must still just throw for the caller's own error handling.
    if (res.status === 401) {
      redirectToLogin()
    }
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error || 'Request failed')
  }

  if (res.status === 204) {
    return undefined as T
  }

  return res.json() as Promise<T>
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body: JSON.stringify(body) }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
}
