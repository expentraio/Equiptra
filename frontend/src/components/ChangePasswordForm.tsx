import { useState, type FormEvent } from 'react'
import { api, ApiError } from '../lib/api'
import { useAuth } from '../context/AuthContext'
import type { CurrentUser } from '../types'

// Shared by the Settings page (self-service) and the forced "set new
// password" screen (ChangeOwnPassword also clears must_change_password and
// issues a fresh session, so both cases are the same request/response
// shape — only the surrounding page chrome differs).
export function ChangePasswordForm({ submitLabel = 'Change password', onSuccess }: { submitLabel?: string; onSuccess?: () => void }) {
  const { updateUser } = useAuth()
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (newPassword !== confirmPassword) {
      setError('New password and confirmation do not match')
      return
    }
    if (newPassword.length < 8) {
      setError('New password must be at least 8 characters')
      return
    }
    setSubmitting(true)
    try {
      const user = await api.patch<CurrentUser>('/users/me/password', {
        current_password: currentPassword,
        new_password: newPassword,
      })
      updateUser(user)
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
      onSuccess?.()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not change password')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-3.5">
      <label className="flex flex-col gap-1.5 text-[13px] font-medium">
        Current password
        <input
          type="password"
          required
          autoFocus
          value={currentPassword}
          onChange={(e) => setCurrentPassword(e.target.value)}
          className="rounded-control border border-border px-3.5 py-2.5 text-[13.5px] outline-none focus:border-teal"
        />
      </label>
      <label className="flex flex-col gap-1.5 text-[13px] font-medium">
        New password
        <input
          type="password"
          required
          minLength={8}
          value={newPassword}
          onChange={(e) => setNewPassword(e.target.value)}
          placeholder="At least 8 characters"
          className="rounded-control border border-border px-3.5 py-2.5 text-[13.5px] outline-none focus:border-teal"
        />
      </label>
      <label className="flex flex-col gap-1.5 text-[13px] font-medium">
        Confirm new password
        <input
          type="password"
          required
          value={confirmPassword}
          onChange={(e) => setConfirmPassword(e.target.value)}
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
        {submitting ? 'Saving…' : submitLabel}
      </button>
    </form>
  )
}
