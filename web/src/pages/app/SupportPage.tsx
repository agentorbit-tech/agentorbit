import { useState } from 'react'
import { Link } from 'react-router-dom'
import { LifeBuoy, Plus } from 'lucide-react'
import { useI18n } from '@/i18n'
import { EmptyState } from '@/components/app/EmptyState'
import { useTickets, useCreateTicket } from '@/hooks/use-support'

export function SupportPage() {
  const { t } = useI18n()
  const { data: tickets, isLoading, isError, refetch } = useTickets()
  const createTicket = useCreateTicket()
  const [open, setOpen] = useState(false)
  const [subject, setSubject] = useState('')
  const [body, setBody] = useState('')

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!subject.trim() || !body.trim()) return
    await createTicket.mutateAsync({ subject: subject.trim(), body: body.trim() })
    setSubject(''); setBody(''); setOpen(false)
  }

  return (
    <div className="p-6 lg:p-8 space-y-6 animate-fade-in-up">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold text-zinc-50 flex items-center gap-2">
          <LifeBuoy size={18} className="text-indigo-400" /> {t.support_title}
        </h1>
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          className="flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-md bg-indigo-500/[0.12] text-indigo-300 hover:bg-indigo-500/[0.2] transition-colors"
        >
          <Plus size={14} /> {t.support_new_ticket}
        </button>
      </div>

      {open && (
        <form onSubmit={submit} className="space-y-3 rounded-lg border border-zinc-900 bg-zinc-950 p-4">
          <input
            value={subject}
            onChange={(e) => setSubject(e.target.value)}
            placeholder={t.support_subject_label}
            className="w-full rounded-md bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:ring-1 focus:ring-indigo-500"
          />
          <textarea
            value={body}
            onChange={(e) => setBody(e.target.value)}
            placeholder={t.support_message_label}
            rows={4}
            className="w-full rounded-md bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:ring-1 focus:ring-indigo-500"
          />
          <button
            type="submit"
            disabled={createTicket.isPending}
            className="text-sm px-3 py-1.5 rounded-md bg-indigo-600 text-white hover:bg-indigo-500 disabled:opacity-50 transition-colors"
          >
            {t.support_create}
          </button>
        </form>
      )}

      {isError && (
        <p className="text-sm text-zinc-500">
          {t.support_failed}{' '}
          <button onClick={() => refetch()} className="text-zinc-300 hover:text-zinc-50 transition-colors">{t.common_retry}</button>
        </p>
      )}

      {isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-11 skeleton-shimmer rounded-lg" />
          ))}
        </div>
      ) : isError ? null : (tickets?.length ?? 0) === 0 ? (
        <EmptyState
          heading={t.support_empty_heading}
          body={t.support_empty_body}
          action={{ label: t.support_new_ticket, onClick: () => setOpen(true) }}
        />
      ) : (
        <ul className="space-y-1">
          {tickets?.map((ticket) => (
            <li key={ticket.id}>
              <Link
                to={`/support/${ticket.id}`}
                className="flex items-center justify-between rounded-md px-3 py-2.5 text-sm text-zinc-300 hover:bg-zinc-800/50 transition-colors"
              >
                <span className="truncate">{ticket.subject}</span>
                <span className={ticket.status === 'open' ? 'text-emerald-400 text-xs' : 'text-zinc-600 text-xs'}>
                  {ticket.status === 'open' ? t.support_status_open : t.support_status_closed}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
