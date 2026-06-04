import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

vi.mock('@/hooks/use-support', () => ({
  useTickets: vi.fn(),
  useCreateTicket: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
}))

import { useTickets } from '@/hooks/use-support'
import { SupportPage } from './SupportPage'
const mockedTickets = vi.mocked(useTickets)

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <SupportPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('SupportPage', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders the empty state when there are no tickets', () => {
    mockedTickets.mockReturnValue({ data: [], isSuccess: true, isLoading: false } as any)
    renderPage()
    expect(screen.getByText(/No tickets yet/)).toBeTruthy()
  })

  it('renders an error banner when the query fails', () => {
    mockedTickets.mockReturnValue({ isError: true, isLoading: false, refetch: vi.fn() } as any)
    renderPage()
    expect(screen.getByText(/Failed to load tickets/)).toBeTruthy()
  })

  it('renders a ticket row with its subject', () => {
    mockedTickets.mockReturnValue({
      data: [{ id: 't1', subject: 'Login broken', status: 'open', created_at: '', updated_at: '' }],
      isSuccess: true, isLoading: false,
    } as any)
    renderPage()
    expect(screen.getByText('Login broken')).toBeTruthy()
  })
})
