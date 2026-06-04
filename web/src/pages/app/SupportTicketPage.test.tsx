import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

vi.mock('@/hooks/use-support', () => ({
  useTicketMessages: vi.fn(),
  usePostMessage: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
}))

import { useTicketMessages } from '@/hooks/use-support'
import { SupportTicketPage } from './SupportTicketPage'
const mockedMessages = vi.mocked(useTicketMessages)

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/support/t1']}>
        <Routes>
          <Route path="/support/:id" element={<SupportTicketPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('SupportTicketPage', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders an error banner when the query fails', () => {
    mockedMessages.mockReturnValue({ isError: true } as any)
    renderPage()
    expect(screen.getByText(/Failed to load messages/)).toBeTruthy()
  })

  it('renders user and staff messages distinctly', () => {
    mockedMessages.mockReturnValue({
      data: [
        { id: 'm1', ticket_id: 't1', author_id: 'u1', body: 'My question', is_staff_reply: false, created_at: '' },
        { id: 'm2', ticket_id: 't1', author_id: 's1', body: 'Our answer', is_staff_reply: true, created_at: '' },
      ],
      isSuccess: true,
    } as any)
    renderPage()
    expect(screen.getByText('My question')).toBeTruthy()
    expect(screen.getByText('Our answer')).toBeTruthy()
    // Staff badge only appears for the staff reply
    expect(screen.getAllByText('Support').length).toBe(1)
  })
})
