import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useMutation } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import { useAuthStore } from '@/store'
import { queryClient } from '@/lib/queryClient'
import { useI18n } from '@/i18n'
import type { Organization, PaginatedResponse } from '@/types/api'

// Probes the session cookie without going through the shared `api` client.
// The shared client treats a 401 from /api/* as "session expired" and forces a
// hard logout + redirect to /login — fine for an authenticated app, but on
// /auth/invite a 401 simply means "anonymous visitor" and must NOT redirect.
async function probeSession(): Promise<boolean> {
  try {
    const res = await fetch('/api/orgs/', {
      method: 'GET',
      credentials: 'same-origin',
      headers: { 'X-Requested-With': 'XMLHttpRequest' },
    })
    return res.ok
  } catch {
    return false
  }
}

const PENDING_INVITE_KEY = 'agentorbit_pending_invite'

export function savePendingInvite(token: string) {
  sessionStorage.setItem(PENDING_INVITE_KEY, token)
}

export function takePendingInvite(): string | null {
  const t = sessionStorage.getItem(PENDING_INVITE_KEY)
  if (t) sessionStorage.removeItem(PENDING_INVITE_KEY)
  return t
}

// Read the pending invite token without consuming it. Used by the register
// page to render in invite-mode while leaving the token available for the
// regular accept-flow if the user navigates away mid-registration.
export function peekPendingInvite(): string | null {
  return sessionStorage.getItem(PENDING_INVITE_KEY)
}

export function clearPendingInvite() {
  sessionStorage.removeItem(PENDING_INVITE_KEY)
}

interface AcceptResponse {
  accepted: boolean
  organization_id: string
}

export function AcceptInvitePage() {
  const navigate = useNavigate()
  const { t } = useI18n()
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') ?? ''
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const setAuthenticated = useAuthStore((s) => s.setAuthenticated)
  const setActiveOrgID = useAuthStore((s) => s.setActiveOrgID)

  // Probe the session cookie on mount: a returning user who lands on /auth/invite
  // after a page refresh has isAuthenticated=false in the persist store but a
  // valid httpOnly cookie. We cannot tell the difference without asking the
  // server. The probe runs once, regardless of token presence, so we render
  // either the auth prompt or the accept flow without flicker.
  const [sessionChecked, setSessionChecked] = useState(isAuthenticated)
  useEffect(() => {
    if (isAuthenticated) {
      setSessionChecked(true)
      return
    }
    let cancelled = false
    probeSession().then((ok) => {
      if (cancelled) return
      if (ok) setAuthenticated(true)
      setSessionChecked(true)
    })
    return () => {
      cancelled = true
    }
  }, [isAuthenticated, setAuthenticated])

  const acceptMutation = useMutation({
    mutationFn: () => api.post<AcceptResponse>('/auth/accept-invite', { token }),
    onSuccess: async (data) => {
      setActiveOrgID(data.organization_id)
      // Refresh org list so the new membership appears in the switcher.
      try {
        await queryClient.refetchQueries({ queryKey: ['orgs'] })
      } catch {
        /* non-fatal */
      }
      // Sync org locale to i18n if available.
      try {
        const orgs = queryClient.getQueryData<Organization[] | PaginatedResponse<Organization>>(['orgs'])
        const list = Array.isArray(orgs) ? orgs : orgs?.data
        const joined = list?.find((o) => o.id === data.organization_id)
        const locale = (joined as (Organization & { locale?: string }) | undefined)?.locale
        if (locale === 'ru' || locale === 'en') useI18n.getState().setLang(locale)
      } catch {
        /* ignore */
      }
      navigate('/dash', { replace: true })
    },
  })

  // Auto-fire accept once the token + auth are present.
  const triggeredRef = useRef(false)
  useEffect(() => {
    if (!token || !isAuthenticated || triggeredRef.current) return
    triggeredRef.current = true
    acceptMutation.mutate()
  }, [token, isAuthenticated]) // eslint-disable-line react-hooks/exhaustive-deps

  // No token at all.
  if (!token) {
    return (
      <Shell>
        <p className="text-sm text-zinc-300">{t.auth_invite_token_invalid}</p>
        <Link to="/login" className="text-sm text-zinc-500 hover:text-zinc-300 mt-4 inline-block">
          {t.auth_verify_back}
        </Link>
      </Shell>
    )
  }

  // Session probe still pending — render nothing to avoid flashing the
  // sign-in prompt at returning users.
  if (!sessionChecked) {
    return (
      <Shell>
        <div className="flex items-center justify-center gap-2 text-zinc-500">
          <Loader2 size={16} className="animate-spin" />
        </div>
      </Shell>
    )
  }

  // Not signed in: stash token and offer sign-in / register.
  if (!isAuthenticated) {
    savePendingInvite(token)
    return (
      <Shell>
        <h1 className="text-2xl font-semibold tracking-[-0.02em] text-zinc-50 mb-2">
          {t.auth_invite_title}
        </h1>
        <p className="text-sm text-zinc-400 mb-6">{t.auth_invite_signin_required}</p>
        <div className="flex flex-col gap-2">
          <Link
            to="/login"
            className="w-full bg-zinc-50 text-zinc-950 py-2.5 rounded-md text-sm font-medium hover:bg-zinc-200 transition-colors text-center"
          >
            {t.auth_invite_signin_btn}
          </Link>
          <Link
            to="/register"
            className="w-full border border-zinc-800 text-zinc-200 py-2.5 rounded-md text-sm font-medium hover:bg-zinc-900 transition-colors text-center"
          >
            {t.auth_invite_register_btn}
          </Link>
        </div>
      </Shell>
    )
  }

  // Authenticated path: pending / success / error.
  if (acceptMutation.isPending || acceptMutation.isIdle) {
    return (
      <Shell>
        <div className="flex items-center justify-center gap-2 text-zinc-400">
          <Loader2 size={16} className="animate-spin" />
          <span className="text-sm">{t.auth_invite_accepting}</span>
        </div>
      </Shell>
    )
  }

  if (acceptMutation.isSuccess) {
    return (
      <Shell>
        <p className="text-sm text-zinc-200">{t.auth_invite_success}</p>
      </Shell>
    )
  }

  // Error.
  const err = acceptMutation.error
  const code = err instanceof ApiError ? err.code : 'unknown'
  let message = t.auth_invite_expired
  if (code === 'email_mismatch') message = t.auth_invite_email_mismatch
  else if (code === 'already_member') message = t.auth_invite_already_member
  else if (code === 'invalid_token' || code === 'token_expired') message = t.auth_invite_expired

  return (
    <Shell>
      <p className="text-sm text-zinc-300 mb-4">{message}</p>
      <div className="flex flex-col gap-2">
        {code === 'email_mismatch' && <LogoutButton label={t.auth_invite_logout_btn} />}
        <Link
          to="/dash"
          className="w-full border border-zinc-800 text-zinc-200 py-2.5 rounded-md text-sm font-medium hover:bg-zinc-900 transition-colors text-center"
        >
          {t.auth_invite_go_dashboard}
        </Link>
      </div>
    </Shell>
  )
}

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen flex items-center justify-center bg-zinc-950 px-6 auth-grid-bg">
      <div className="w-full max-w-sm text-center">
        <div className="mb-6">
          <span className="text-sm font-semibold text-zinc-50 tracking-tight">AgentOrbit</span>
        </div>
        {children}
      </div>
    </div>
  )
}

function LogoutButton({ label }: { label: string }) {
  const navigate = useNavigate()
  const logout = useAuthStore((s) => s.logout)
  return (
    <button
      type="button"
      onClick={async () => {
        try {
          await fetch('/auth/logout', {
            method: 'POST',
            credentials: 'same-origin',
            headers: { 'X-Requested-With': 'XMLHttpRequest' },
          })
        } catch {
          /* ignore */
        }
        logout()
        navigate('/login', { replace: true })
      }}
      className="w-full bg-zinc-50 text-zinc-950 py-2.5 rounded-md text-sm font-medium hover:bg-zinc-200 transition-colors"
    >
      {label}
    </button>
  )
}
