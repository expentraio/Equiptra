import { useEffect, useState } from 'react'
import { api, ApiError } from '../lib/api'
import { useAuth } from '../context/AuthContext'
import type { User, UserRole } from '../types'
import { PlusIcon } from '../components/icons'
import { NewUserModal } from '../components/NewUserModal'

export function Users() {
  const { user: currentUser } = useAuth()
  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(true)
  const [showNew, setShowNew] = useState(false)
  const [rowError, setRowError] = useState<{ id: number; message: string } | null>(null)

  function reload() {
    setLoading(true)
    api
      .get<User[]>('/users')
      .then(setUsers)
      .finally(() => setLoading(false))
  }

  useEffect(reload, [])

  async function toggleActive(u: User) {
    setRowError(null)
    try {
      await api.patch<User>(`/users/${u.id}`, { active: !u.active })
      reload()
    } catch (err) {
      setRowError({ id: u.id, message: err instanceof ApiError ? err.message : 'Could not update user' })
    }
  }

  async function changeRole(u: User, role: UserRole) {
    setRowError(null)
    try {
      await api.patch<User>(`/users/${u.id}`, { role })
      reload()
    } catch (err) {
      setRowError({ id: u.id, message: err instanceof ApiError ? err.message : 'Could not update user' })
    }
  }

  async function deleteUser(u: User) {
    if (!confirm(`Permanently delete ${u.name}? This cannot be undone.`)) return
    setRowError(null)
    try {
      await api.delete(`/users/${u.id}`)
      reload()
    } catch (err) {
      setRowError({ id: u.id, message: err instanceof ApiError ? err.message : 'Could not delete user' })
    }
  }

  return (
    <div>
      <div className="mb-5.5 flex flex-wrap items-start justify-between gap-5">
        <div>
          <h1 className="text-[21px] font-bold">Users</h1>
          <p className="mt-1 text-[13px] text-ink-soft">Who can log in, and what they can do</p>
        </div>
        <button
          onClick={() => setShowNew(true)}
          className="flex items-center gap-1.5 rounded-control bg-teal px-4 py-2.25 text-[13px] font-medium text-white hover:opacity-90"
        >
          <PlusIcon className="h-4 w-4" />
          New user
        </button>
      </div>

      <div className="overflow-hidden rounded-card border border-border bg-surface">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[620px] border-collapse">
            <thead>
              <tr>
                {['Name', 'Email', 'Role', 'Status', ''].map((h) => (
                  <th
                    key={h}
                    className="border-b border-border px-4 py-3 text-left text-[11px] font-semibold uppercase tracking-[.05em] text-ink-soft"
                  >
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {users.map((u) => {
                const isSelf = u.id === currentUser?.id
                return (
                  <tr key={u.id}>
                    <td className="border-b border-border px-4 py-3.25 text-[13px] font-medium">
                      {u.name}
                      {isSelf && <span className="ml-1.5 text-[11px] font-normal text-ink-soft">(you)</span>}
                    </td>
                    <td className="border-b border-border px-4 py-3.25 text-[13px] text-ink-soft">{u.email}</td>
                    <td className="border-b border-border px-4 py-3.25 text-[13px]">
                      <select
                        value={u.role}
                        onChange={(e) => void changeRole(u, e.target.value as UserRole)}
                        className="rounded-control border border-border bg-surface px-2 py-1.5 text-[12.5px] outline-none focus:border-teal"
                      >
                        <option value="standard">Standard</option>
                        <option value="admin">Admin</option>
                      </select>
                    </td>
                    <td className="border-b border-border px-4 py-3.25 text-[13px]">
                      <span
                        className={`rounded-full px-2.5 py-1 text-[11px] font-semibold ${
                          u.active ? 'bg-teal-fill text-teal' : 'bg-[#EDEBE4] text-ink-soft'
                        }`}
                      >
                        {u.active ? 'active' : 'inactive'}
                      </span>
                    </td>
                    <td className="border-b border-border px-4 py-3.25 text-right text-[12px]">
                      <div className="flex items-center justify-end gap-3">
                        <button
                          onClick={() => void toggleActive(u)}
                          disabled={isSelf && u.active}
                          title={isSelf && u.active ? 'Cannot deactivate your own account' : undefined}
                          className="font-medium text-ink-soft hover:text-ink disabled:cursor-not-allowed disabled:text-[#C9C5BA] disabled:hover:text-[#C9C5BA]"
                        >
                          {u.active ? 'Deactivate' : 'Activate'}
                        </button>
                        <button
                          onClick={() => void deleteUser(u)}
                          disabled={isSelf || u.has_allocation_history}
                          title={
                            isSelf
                              ? 'Cannot delete your own account'
                              : u.has_allocation_history
                                ? 'This user has checkout/checkin history — deactivate instead'
                                : undefined
                          }
                          className="font-medium text-ink-soft hover:text-red disabled:cursor-not-allowed disabled:text-[#C9C5BA] disabled:hover:text-[#C9C5BA]"
                        >
                          Delete
                        </button>
                      </div>
                      {rowError?.id === u.id && <div className="mt-1.5 text-[11px] font-medium text-red">{rowError.message}</div>}
                    </td>
                  </tr>
                )
              })}
              {!loading && users.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-[13px] text-ink-soft">
                    No users yet.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {showNew && (
        <NewUserModal
          onClose={() => setShowNew(false)}
          onCreated={() => {
            setShowNew(false)
            reload()
          }}
        />
      )}
    </div>
  )
}
