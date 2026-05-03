import { createBrowserRouter, RouterProvider, Outlet, Navigate } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { queryClient } from '@/lib/queryClient'
import { useMeta } from '@/hooks/use-meta'
import { AuthGuard } from '@/components/app/AuthGuard'
import { AppShell } from '@/components/app/AppShell'
import { PublicLayout } from '@/components/app/PublicLayout'
import { LoginPage } from '@/pages/public/LoginPage'
import { RegisterPage } from '@/pages/public/RegisterPage'
import { VerifyEmailPage } from '@/pages/public/VerifyEmailPage'
import { RequestPasswordResetPage } from '@/pages/public/RequestPasswordResetPage'
import { ResetPasswordPage } from '@/pages/public/ResetPasswordPage'
import { AcceptInvitePage } from '@/pages/public/AcceptInvitePage'
import { LandingPage } from '@/pages/public/LandingPage'
import { PrivacyPolicyPage } from '@/pages/public/PrivacyPolicyPage'
import { TermsPage } from '@/pages/public/TermsPage'
import { ConsentPage } from '@/pages/public/ConsentPage'
import { CreateOrgPage } from '@/pages/app/CreateOrgPage'
import { DashboardPage } from '@/pages/app/DashboardPage'
import { SessionsPage } from '@/pages/app/SessionsPage'
import { SessionDetailPage } from '@/pages/app/SessionDetailPage'
import { APIKeysPage } from '@/pages/app/APIKeysPage'
import { SettingsPage } from '@/pages/app/SettingsPage'
import { SystemPromptsPage } from '@/pages/app/SystemPromptsPage'
import { SystemPromptDetailPage } from '@/pages/app/SystemPromptDetailPage'
import { FailureClustersPage } from '@/pages/app/FailureClustersPage'
import { RouteErrorBoundary, NotFoundPage } from '@/components/app/ErrorBoundary'

// Landing page is cloud-only. In self-host (no BILLING_URL), redirect / to /login.
// While /meta is in flight we render nothing to avoid flashing the landing page
// for self-host users. This component also acts as the de-facto /meta prefetch
// for the entire app — the route mounts on first nav, and `useBillingEnabled`
// downstream reads from the same query cache.
function LandingOrLogin() {
  const meta = useMeta()
  if (meta.isLoading || !meta.isFetched) return null
  return meta.data?.billing_url ? <LandingPage /> : <Navigate to="/login" replace />
}

const router = createBrowserRouter([
  {
    element: <Outlet />,
    errorElement: <RouteErrorBoundary />,
    children: [
      {
        element: <PublicLayout />,
        children: [
          { path: '/', element: <LandingOrLogin /> },
          { path: '/login', element: <LoginPage /> },
          { path: '/register', element: <RegisterPage /> },
          { path: '/verify-email', element: <VerifyEmailPage /> },
          { path: '/request-password-reset', element: <RequestPasswordResetPage /> },
          { path: '/reset-password', element: <ResetPasswordPage /> },
          { path: '/auth/invite', element: <AcceptInvitePage /> },
          { path: '/privacy-policy', element: <PrivacyPolicyPage /> },
          { path: '/terms', element: <TermsPage /> },
          { path: '/consent', element: <ConsentPage /> },
          { path: '*', element: <NotFoundPage /> },
        ],
      },
      {
        element: <AuthGuard />,
        children: [
          {
            element: <PublicLayout />,
            children: [{ path: '/create-org', element: <CreateOrgPage /> }],
          },
          {
            element: <AppShell />,
            children: [
              { path: '/dash', element: <DashboardPage /> },
              { path: '/sessions', element: <SessionsPage /> },
              { path: '/sessions/:id', element: <SessionDetailPage /> },
              { path: '/keys', element: <APIKeysPage /> },
              { path: '/system-prompts', element: <SystemPromptsPage /> },
              { path: '/system-prompts/:id', element: <SystemPromptDetailPage /> },
              { path: '/failure-clusters', element: <FailureClustersPage /> },
              { path: '/settings', element: <SettingsPage /> },
            ],
          },
        ],
      },
    ],
  },
])

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}
