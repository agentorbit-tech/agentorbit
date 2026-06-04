import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useI18n } from '@/i18n'
import { useTicketMessages, usePostMessage } from '@/hooks/use-support'

export function SupportTicketPage() {
  const { t } = useI18n()
  const { id = '' } = useParams()
  const { data: messages, isError, refetch } = useTicketMessages(id)
  const postMessage = usePostMessage(id)
  const [draft, setDraft] = useState('')

  async function send(e: React.FormEvent) {
    e.preventDefault()
    if (!draft.trim()) return
    await postMessage.mutateAsync({ body: draft.trim() })
    setDraft('')
  }

  return (
    <div className="p-6 lg:p-8 space-y-4 animate-fade-in-up max-w-3xl">
      <Link to="/support" className="inline-flex items-center gap-1.5 text-sm text-zinc-500 hover:text-zinc-300 transition-colors">
        <ArrowLeft size={14} /> {t.support_back}
      </Link>

      {isError && (
        <p className="text-sm text-zinc-500">
          {t.support_messages_failed}{' '}
          <button onClick={() => refetch()} className="text-zinc-300 hover:text-zinc-50 transition-colors">{t.common_retry}</button>
        </p>
      )}

      <div className="space-y-3">
        {messages?.map((m) => (
          <div key={m.id} className={cn('flex', m.is_staff_reply ? 'justify-start' : 'justify-end')}>
            <div
              className={cn(
                'max-w-[80%] rounded-lg px-3 py-2 text-sm',
                m.is_staff_reply ? 'bg-zinc-800 text-zinc-200' : 'bg-indigo-600 text-white'
              )}
            >
              {m.is_staff_reply && (
                <span className="block text-[10px] uppercase tracking-wide text-indigo-300 mb-0.5">{t.support_staff_badge}</span>
              )}
              {m.body}
            </div>
          </div>
        ))}
      </div>

      <form onSubmit={send} className="flex gap-2">
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder={t.support_reply_placeholder}
          className="flex-1 rounded-md bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:ring-1 focus:ring-indigo-500"
        />
        <button
          type="submit"
          disabled={postMessage.isPending}
          className="text-sm px-4 py-2 rounded-md bg-indigo-600 text-white hover:bg-indigo-500 disabled:opacity-50 transition-colors"
        >
          {t.support_send}
        </button>
      </form>
    </div>
  )
}
