import { useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2, Sparkles, Check } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import { useI18n } from '@/i18n'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { useBillingEnabled } from '@/hooks/use-meta'
import { useCreateProRequest, useProRequestStatus } from '@/hooks/use-billing'

const inputClass = "w-full bg-zinc-900 border border-zinc-800 rounded-md px-3 py-2.5 text-sm text-zinc-100 placeholder:text-zinc-600 focus:outline-none focus:border-zinc-600 focus:ring-1 focus:ring-zinc-600 transition-colors"

interface CurrentUser {
  id: string
  email: string
  name: string
}

function useCurrentUser() {
  return useQuery<CurrentUser>({
    queryKey: ['user', 'me'],
    queryFn: () => api.get<CurrentUser>('/api/user/me'),
    staleTime: 5 * 60 * 1000,
  })
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  source: string
}

export function ProRequestDialog({ open, onOpenChange, source }: Props) {
  const { t } = useI18n()
  const enabled = useBillingEnabled()
  const status = useProRequestStatus()
  const me = useCurrentUser()
  const createPR = useCreateProRequest()

  const [company, setCompany] = useState('')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  const alreadyRequested = status.data?.requested === true
  const isSuccess = createPR.isSuccess || alreadyRequested

  useEffect(() => {
    if (open) return
    // Reset after the dialog has finished its close animation. Cleanup
    // prevents a stale reset from clobbering state if the dialog is
    // re-opened before the timer fires.
    const id = window.setTimeout(() => {
      setCompany('')
      setMessage('')
      setError('')
      createPR.reset()
    }, 200)
    return () => window.clearTimeout(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    // Guarded: the submit button is disabled until me.data.email is loaded.
    const email = me.data?.email
    if (!email) return
    setError('')
    createPR.mutate(
      { email, company, message, source },
      {
        onError: (err) => {
          if (err instanceof ApiError && err.status === 409) {
            // 409 -> caller cache update flips alreadyRequested true; render success state.
            return
          }
          setError(t.pro_dialog_error)
        },
      },
    )
  }

  if (!enabled) return null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="bg-zinc-950 border-zinc-800 max-w-md">
        <DialogHeader>
          <DialogTitle className="text-zinc-50">{t.pro_dialog_title}</DialogTitle>
        </DialogHeader>

        {isSuccess ? (
          <div className="text-center py-6">
            <div className="inline-flex items-center justify-center w-10 h-10 rounded-full bg-emerald-500/10 mb-3">
              <Check size={18} className="text-emerald-500" />
            </div>
            <p className="text-sm text-zinc-300">{t.pro_dialog_success}</p>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4 pt-2">
            <p className="text-sm text-zinc-500">{t.pro_dialog_desc}</p>

            <div>
              <label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.pro_dialog_company}</label>
              <input type="text" value={company} onChange={(e) => setCompany(e.target.value)} placeholder={t.pro_dialog_company_placeholder} className={inputClass} />
            </div>

            <div>
              <label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.pro_dialog_message}</label>
              <textarea value={message} onChange={(e) => setMessage(e.target.value)} placeholder={t.pro_dialog_message_placeholder} rows={3} className={inputClass + ' resize-none'} />
            </div>

            {error && <p className="text-sm text-red-400">{error}</p>}

            <Button type="submit" disabled={createPR.isPending || !me.data?.email} className="w-full bg-zinc-50 text-zinc-950 hover:bg-zinc-200">
              {createPR.isPending ? <><Loader2 size={14} className="animate-spin mr-2" />{t.pro_dialog_submitting}</> : t.pro_dialog_submit}
            </Button>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}

interface ProCTACardProps {
  title: string
  description: string
  source: string
}

export function ProCTACard({ title, description, source }: ProCTACardProps) {
  const { t } = useI18n()
  const enabled = useBillingEnabled()
  const [open, setOpen] = useState(false)

  if (!enabled) return null

  return (
    <>
      <div className="text-center py-16 max-w-sm mx-auto">
        <div className="inline-flex items-center justify-center w-10 h-10 rounded-full bg-indigo-500/10 mb-4">
          <Sparkles size={18} className="text-indigo-400" />
        </div>
        <h3 className="text-base font-medium text-zinc-200 mb-2">{title}</h3>
        <p className="text-sm text-zinc-500 mb-6 leading-relaxed">{description}</p>
        <button
          onClick={() => setOpen(true)}
          className="inline-flex items-center gap-2 text-sm font-medium bg-indigo-500/10 text-indigo-400 border border-indigo-500/20 px-4 py-2 rounded-md hover:bg-indigo-500/20 transition-colors"
        >
          <Sparkles size={13} />
          {t.pro_cta_btn}
        </button>
      </div>
      <ProRequestDialog open={open} onOpenChange={setOpen} source={source} />
    </>
  )
}
