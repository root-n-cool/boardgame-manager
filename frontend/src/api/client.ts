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
  // The pathname check stops the login page from reloading itself: a wrong-
  // password attempt on LoginView is a POST /login that legitimately 401s
  // while already on /login, and force-navigating to the page already shown
  // would just reload it instead of letting the form show the error.
  //
  // Separately, checkStatus()'s own GET /me passes skipAuthRedirect below: a
  // 401 there is always the expected "nobody's signed in" answer, not a
  // session loss, and checkStatus() now runs on every route — including the
  // public ones — not just /login, so this function can no longer rely on
  // that call only ever happening on the login page. Every other 401 in the
  // app still wants this hard redirect.
  if (redirecting || window.location.pathname === LOGIN_PATH) {
    return
  }
  redirecting = true
  window.location.href = LOGIN_PATH
}

async function request<T>(
  path: string,
  options: RequestInit & { skipAuthRedirect?: boolean } = {},
): Promise<T> {
  const { skipAuthRedirect, ...fetchOptions } = options
  const isFormData = fetchOptions.body instanceof FormData
  const headers: Record<string, string> = {
    ...(isFormData ? {} : { 'Content-Type': 'application/json' }),
    ...((fetchOptions.headers as Record<string, string>) || {}),
  }

  const res = await fetch(`${BASE_URL}${path}`, {
    ...fetchOptions,
    credentials: 'include',
    headers,
  })

  if (!res.ok) {
    // Only 401 means "your session is gone"; a 404 or 409 from a normal call
    // must still just throw for the caller's own error handling. skipAuthRedirect
    // lets a specific call opt out of the hard redirect below (see checkStatus()).
    if (res.status === 401 && !skipAuthRedirect) {
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
  get: <T>(path: string, options?: { skipAuthRedirect?: boolean }) => request<T>(path, options),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, {
      method: 'POST',
      body: body instanceof FormData ? body : body ? JSON.stringify(body) : undefined,
    }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body: JSON.stringify(body) }),
  patch: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PATCH', body: JSON.stringify(body) }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
}
