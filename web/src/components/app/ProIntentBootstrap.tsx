import { useEffect, useState } from 'react'
import { ProRequestDialog } from './ProRequestDialog'
import { useBillingEnabled } from '@/hooks/use-meta'

const INTENT_STORAGE_KEY = 'agentorbit_pro_intent'

// Reads the post-register intent flag set by RegisterPage and auto-opens the
// pro-request dialog once the user lands inside the authenticated app shell.
//
// The dialog component is only mounted after the intent is detected — that
// way we don't pay for its child queries (`/api/user/me`, `/pro-request/me`)
// on every dashboard page for users who didn't come from the landing CTA.
export function ProIntentBootstrap() {
  const enabled = useBillingEnabled()
  const [armed, setArmed] = useState(false)
  const [open, setOpen] = useState(false)

  useEffect(() => {
    if (!enabled) return
    if (sessionStorage.getItem(INTENT_STORAGE_KEY) === '1') {
      sessionStorage.removeItem(INTENT_STORAGE_KEY)
      setArmed(true)
      setOpen(true)
    }
  }, [enabled])

  if (!armed) return null
  return <ProRequestDialog open={open} onOpenChange={setOpen} source="landing_post_register" />
}
