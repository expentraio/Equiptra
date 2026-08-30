import { useState, type FormEvent } from 'react'
import { api, ApiError } from '../lib/api'
import type { Project } from '../types'
import { CloseIcon } from './icons'

export function NewProjectModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState('')
  const [client, setClient] = useState('')
  const [startDate, setStartDate] = useState('')
  const [endDate, setEndDate] = useState('')
  const [carnetRequired, setCarnetRequired] = useState(false)
  const [clientReference, setClientReference] = useState('')
  const [orderNumber, setOrderNumber] = useState('')
  const [deliveryAddress, setDeliveryAddress] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      await api.post<Project>('/projects', {
        name,
        client: client || null,
        start_date: new Date(startDate).toISOString(),
        end_date: new Date(endDate).toISOString(),
        status: 'tentative',
        carnet_required: carnetRequired,
        client_reference: clientReference || null,
        order_number: orderNumber || null,
        delivery_address: deliveryAddress || null,
      })
      onCreated()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not create project')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[rgba(15,23,42,.35)] p-4" onClick={onClose}>
      <form
        onSubmit={handleSubmit}
        onClick={(e) => e.stopPropagation()}
        className="max-h-[90vh] w-full max-w-[440px] overflow-y-auto rounded-card border border-border bg-surface p-6"
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-[16px] font-bold">New project</h2>
          <button type="button" onClick={onClose} className="p-1 text-ink-soft hover:text-ink">
            <CloseIcon className="h-4.5 w-4.5" />
          </button>
        </div>

        <div className="flex flex-col gap-3.5">
          <label className="flex flex-col gap-1.5 text-[13px] font-medium">
            Name
            <input
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
            />
          </label>
          <label className="flex flex-col gap-1.5 text-[13px] font-medium">
            Client
            <input
              value={client}
              onChange={(e) => setClient(e.target.value)}
              className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
            />
          </label>
          <div className="flex gap-3">
            <label className="flex flex-1 flex-col gap-1.5 text-[13px] font-medium">
              Start date
              <input
                type="date"
                required
                value={startDate}
                onChange={(e) => setStartDate(e.target.value)}
                className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
              />
            </label>
            <label className="flex flex-1 flex-col gap-1.5 text-[13px] font-medium">
              End date
              <input
                type="date"
                required
                value={endDate}
                onChange={(e) => setEndDate(e.target.value)}
                className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
              />
            </label>
          </div>

          <div className="mt-1 text-[11px] font-semibold uppercase tracking-[.06em] text-ink-soft">
            For paperwork (carnet / delivery note)
          </div>
          <div className="flex gap-3">
            <label className="flex flex-1 flex-col gap-1.5 text-[13px] font-medium">
              Client reference
              <input
                value={clientReference}
                onChange={(e) => setClientReference(e.target.value)}
                placeholder="Their PO/job ref"
                className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
              />
            </label>
            <label className="flex flex-1 flex-col gap-1.5 text-[13px] font-medium">
              Order number
              <input
                value={orderNumber}
                onChange={(e) => setOrderNumber(e.target.value)}
                placeholder="e.g. 536-244"
                className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
              />
            </label>
          </div>
          <label className="flex flex-col gap-1.5 text-[13px] font-medium">
            Delivery address
            <textarea
              value={deliveryAddress}
              onChange={(e) => setDeliveryAddress(e.target.value)}
              rows={3}
              placeholder={'One line per address line'}
              className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
            />
          </label>

          <label className="flex items-center gap-2 text-[13px] font-medium">
            <input type="checkbox" checked={carnetRequired} onChange={(e) => setCarnetRequired(e.target.checked)} />
            Carnet required (reminder flag only)
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
            {submitting ? 'Creating…' : 'Create project'}
          </button>
        </div>
      </form>
    </div>
  )
}
