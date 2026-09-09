// Quickstart: login-walled live list (copy-paste, <10 min against self-hosted Openbase).
import { createClient } from '@openbase/js'

const openbase = createClient(process.env.OPENBASE_URL!, process.env.OPENBASE_ANON_KEY!)

export async function login(email: string, password: string) {
  const { data, error } = await openbase.auth.signInWithPassword({ email, password })
  if (error) throw new Error(error.message)
  return data.session
}

export async function liveOpenOrders(onRow: (row: unknown) => void) {
  const { data, error } = await openbase.from('orders').select('id,status,total').eq('status', 'open').limit(20)
  if (error) throw new Error(error.message)
  for (const row of data as unknown[]) onRow(row)
  return openbase
    .channel('orders-feed')
    .on('postgres_changes', { event: '*', table: 'orders', filter: 'status=eq.open' }, (change) => onRow(change.data))
    .subscribe()
}
