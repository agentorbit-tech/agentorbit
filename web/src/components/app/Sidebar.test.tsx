import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Sidebar } from './Sidebar'

vi.mock('@/hooks/use-meta', () => ({ useBillingEnabled: vi.fn() }))
vi.mock('@/hooks/use-websocket', () => ({ useWSStore: vi.fn(() => 'connected') }))
vi.mock('@/store', () => ({ useAuthStore: vi.fn(() => () => {}) }))
vi.mock('./OrgSwitcher', () => ({ OrgSwitcher: () => null }))
vi.mock('./UsageBar', () => ({ UsageBar: () => null }))

import { useBillingEnabled } from '@/hooks/use-meta'
const mockedBilling = vi.mocked(useBillingEnabled)

function renderSidebar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <Sidebar collapsed={false} onToggle={() => {}} mobileOpen={false} onMobileClose={() => {}} />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('Sidebar support entry', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows Support when billing is enabled', () => {
    mockedBilling.mockReturnValue(true)
    renderSidebar()
    const link = screen.getAllByText('Support')[0]
    expect(link.closest('a')?.getAttribute('href')).toBe('/support')
  })

  it('hides Support when billing is disabled (self-host)', () => {
    mockedBilling.mockReturnValue(false)
    renderSidebar()
    expect(screen.queryByText('Support')).toBeNull()
  })
})
