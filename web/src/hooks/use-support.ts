import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, BillingNotConfigured, billingApi } from '@/lib/api'
import { useBillingEnabled } from '@/hooks/use-meta'

export type TicketStatus = 'open' | 'closed'

export interface Ticket {
  id: string
  subject: string
  status: TicketStatus
  created_at: string
  updated_at: string
}

export interface TicketMessage {
  id: string
  ticket_id: string
  author_id: string
  body: string
  is_staff_reply: boolean
  created_at: string
}

const TICKETS_KEY = ['support', 'tickets'] as const
const messagesKey = (id: string) => ['support', 'tickets', id, 'messages'] as const

// Billing 401 means the JWT cookie hasn't propagated yet; don't retry. Same for
// BillingNotConfigured (gated by `enabled`, defensive). Mirrors use-billing.ts.
export function billingRetry(count: number, err: unknown): boolean {
  if (err instanceof BillingNotConfigured) return false
  if (err instanceof ApiError && err.status === 401) return false
  return count < 1
}

export function useTickets() {
  const enabled = useBillingEnabled()
  return useQuery<Ticket[], Error>({
    queryKey: TICKETS_KEY,
    queryFn: () => billingApi.get<Ticket[]>('/support/tickets'),
    enabled,
    retry: billingRetry,
    staleTime: 10_000,
  })
}

export function useTicketMessages(ticketId: string) {
  const billingEnabled = useBillingEnabled()
  return useQuery<TicketMessage[], Error>({
    queryKey: messagesKey(ticketId),
    queryFn: () => billingApi.get<TicketMessage[]>(`/support/tickets/${ticketId}/messages`),
    enabled: billingEnabled && ticketId.length > 0,
    retry: billingRetry,
    refetchInterval: 5_000,
  })
}

export interface CreateTicketBody {
  subject: string
  body: string
}

export function useCreateTicket() {
  const qc = useQueryClient()
  return useMutation<Ticket, ApiError, CreateTicketBody>({
    mutationFn: (body) => billingApi.post<Ticket>('/support/tickets', body),
    onSuccess: () => { qc.invalidateQueries({ queryKey: TICKETS_KEY }) },
  })
}

export interface PostMessageBody {
  body: string
}

export function usePostMessage(ticketId: string) {
  const qc = useQueryClient()
  return useMutation<TicketMessage, ApiError, PostMessageBody>({
    mutationFn: (body) => billingApi.post<TicketMessage>(`/support/tickets/${ticketId}/messages`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: messagesKey(ticketId) })
      qc.invalidateQueries({ queryKey: TICKETS_KEY })
    },
  })
}
