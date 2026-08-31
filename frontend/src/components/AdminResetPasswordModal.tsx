import { useState, type FormEvent } from 'react'
import { api, ApiError } from '../lib/api'
import type { User } from '../types'
import { CloseIcon } from './icons'

export function AdminResetPasswordModal({ user, onClose, onDone }: { user: User; onClose: () => void; onDone: () => void }) {
  const [newPassword, setNewPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [done, setDone] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (newPassword.length < 8) {
      setError('Password must be at least 8 characters')
      return
    }
    setSubmitting(true)
    try {
      await api.patch(`/users/${user.id}/password`, { new_password: newPassword })
      setDone(true)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not reset password')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[rgba(15,23,42,.35)] p-4" onClick={onClose}>
      <div onClick={(e) => e.stopPropagation()} className="w-full max-w-[400px] rounded-card border border-border bg-surface p-6">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-[16px] font-bold">Reset password — {user.name}</h2>
          <button type="button" onClick={done ? onDone : onClose} className="p-1 text-ink-soft hover:text-ink">
            <CloseIcon className="h-4.5 w-4.5" />
          </button>
        </div>

        {done ? (
          <div className="flex flex-col gap-3.5">
            <div className="rounded-control border border-teal-fill bg-teal-fill px-3.5 py-2.5 text-[13px] font-medium text-teal">
              Password reset. {user.name} will be asked to set their own password next time they log in.
            </div>
            <p className="text-[12.5px] text-ink-soft">
              Relay this temporary password to them directly (Slack, in person) — it isn't shown anywhere else.
            </p>
            <div className="rounded-control border border-border bg-off-white px-3.5 py-2.5 font-mono text-[13.5px]">
              {newPassword}
            </div>
            <button
              type="button"
              onClick={onDone}
              className="mt-1 rounded-control bg-teal px-4 py-2.5 text-[13px] font-medium text-white hover:opacity-90"
            >
              Done
            </button>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="flex flex-col gap-3.5">
            <p className="text-[12.5px] text-ink-soft">
              Sets a temporary password for {user.name}. They'll be required to set their own new password the next time
              they log in.
            </p>
            <label className="flex flex-col gap-1.5 text-[13px] font-medium">
              Temporary password
              <input
                type="password"
                required
                autoFocus
                minLength={8}
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                placeholder="At least 8 characters"
                className="rounded-control border border-border px-3.5 py-2.5 text-[13.5px] outline-none focus:border-teal"
              />
            </label>

            {error && (
              <div className="rounded-control border border-red-fill bg-red-fill px-3.5 py-2.5 text-[13px] font-medium text-red">
                {error}
              </div>
            )}

            <button
              type="submit"
              disabled={submitting}
              className="mt-1 rounded-control bg-teal px-4 py-2.5 text-[13px] font-medium text-white hover:opacity-90 disabled:opacity-60"
            >
              {submitting ? 'Resetting…' : 'Reset password'}
            </button>
          </form>
        )}
      </div>
    </div>
  )
}
