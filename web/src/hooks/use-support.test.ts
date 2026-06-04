import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import React from 'react'

vi.mock('@/lib/api', () => ({
  billingApi: { get: vi.fn(), post: vi.fn() },
  ApiError: class ApiError extends Error {
    status: number
    code: string
    constructor(status: number, code: string, message: string) {
      super(message); this.status = status; this.code = code
    }
  },
  BillingNotConfigured: class BillingNotConfigured extends Error {},
}))
vi.mock('@/hooks/use-meta', () => ({ useBillingEnabled: vi.fn(() => true) }))

import { billingApi, ApiError, BillingNotConfigured } from '@/lib/api'
import { useTickets, useTicketMessages, useCreateTicket, usePostMessage, billingRetry } from './use-support'

const mockedGet = vi.mocked(billingApi.get)

function wrapper({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return React.createElement(QueryClientProvider, { client: qc }, children)
}

describe('useTickets', () => {
  beforeEach(() => vi.clearAllMocks())

  it('fetches the ticket list from billing', async () => {
    mockedGet.mockResolvedValue([
      { id: 't1', subject: 'Help', status: 'open', updated_at: '2026-06-04T00:00:00Z', created_at: '2026-06-04T00:00:00Z' },
    ])
    const { result } = renderHook(() => useTickets(), { wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockedGet).toHaveBeenCalledWith('/support/tickets')
    expect(result.current.data?.[0].subject).toBe('Help')
  })
})

describe('billingRetry', () => {
  it('does not retry on BillingNotConfigured', () => {
    expect(billingRetry(0, new BillingNotConfigured())).toBe(false)
  })
  it('does not retry on 401', () => {
    expect(billingRetry(0, new ApiError(401, 'unauthorized', 'x'))).toBe(false)
  })
  it('retries once on other errors', () => {
    expect(billingRetry(0, new Error('boom'))).toBe(true)
    expect(billingRetry(1, new Error('boom'))).toBe(false)
  })
})

describe('useTicketMessages', () => {
  beforeEach(() => vi.clearAllMocks())
  it('polls messages for a ticket', async () => {
    mockedGet.mockResolvedValue([
      { id: 'm1', ticket_id: 't1', author_id: 'u1', body: 'hi', is_staff_reply: false, created_at: '2026-06-04T00:00:00Z' },
    ])
    const { result } = renderHook(() => useTicketMessages('t1'), { wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockedGet).toHaveBeenCalledWith('/support/tickets/t1/messages')
    expect(result.current.data?.[0].body).toBe('hi')
  })
  it('is disabled when ticketId is empty', () => {
    const { result } = renderHook(() => useTicketMessages(''), { wrapper })
    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useCreateTicket', () => {
  beforeEach(() => vi.clearAllMocks())
  it('POSTs subject and body', async () => {
    const mockedPost = vi.mocked(billingApi.post)
    mockedPost.mockResolvedValue({ id: 't9', subject: 'X', status: 'open', created_at: '', updated_at: '' })
    const { result } = renderHook(() => useCreateTicket(), { wrapper })
    await act(async () => { await result.current.mutateAsync({ subject: 'X', body: 'B' }) })
    expect(mockedPost).toHaveBeenCalledWith('/support/tickets', { subject: 'X', body: 'B' })
  })
})

describe('usePostMessage', () => {
  beforeEach(() => vi.clearAllMocks())
  it('POSTs a message to the ticket', async () => {
    const mockedPost = vi.mocked(billingApi.post)
    mockedPost.mockResolvedValue({ id: 'm2', ticket_id: 't1', author_id: 'u1', body: 'reply', is_staff_reply: false, created_at: '' })
    const { result } = renderHook(() => usePostMessage('t1'), { wrapper })
    await act(async () => { await result.current.mutateAsync({ body: 'reply' }) })
    expect(mockedPost).toHaveBeenCalledWith('/support/tickets/t1/messages', { body: 'reply' })
  })
})
