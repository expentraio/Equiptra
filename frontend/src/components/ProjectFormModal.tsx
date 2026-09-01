import { useState, type FormEvent } from 'react'
import { api, ApiError } from '../lib/api'
import type { Project } from '../types'
import { CloseIcon } from './icons'

function toDateInput(iso: string) {
  return iso.slice(0, 10)
}

export function ProjectFormModal({
  project,
  onClose,
  onSaved,
}: {
  project?: Project
  onClose: () => void
  onSaved: () => void
}) {
  const isEdit = !!project
  const [name, setName] = useState(project?.name ?? '')
  const [client, setClient] = useState(project?.client ?? '')
  const [startDate, setStartDate] = useState(project ? toDateInput(project.start_date) : '')
  const [endDate, setEndDate] = useState(project ? toDateInput(project.end_date) : '')
  const [carnetRequired, setCarnetRequired] = useState(project?.carnet_required ?? false)
  const [clientReference, setClientReference] = useState(project?.client_reference ?? '')
  const [orderNumber, setOrderNumber] = useState(project?.order_number ?? '')
  const [deliveryAddress, setDeliveryAddress] = useState(project?.delivery_address ?? '')
  const [notes, setNotes] = useState(project?.notes ?? '')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      const body = {
        name,
        client: client || null,
        start_date: new Date(startDate).toISOString(),
        end_date: new Date(endDate).toISOString(),
        carnet_required: carnetRequired,
        client_reference: clientReference || null,
        order_number: orderNumber || null,
        delivery_address: deliveryAddress || null,
        notes: notes || null,
      }
      if (isEdit) {
        await api.put(`/projects/${project.id}`, body)
      } else {
        await api.post('/projects', body)
      }
      onSaved()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not save project')
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
          <h2 className="text-[16px] font-bold">{isEdit ? 'Edit project' : 'New project'}</h2>
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
          <label className="flex flex-col gap-1.5 text-[13px] font-medium">
            Notes
            <textarea
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              rows={3}
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
            {submitting ? 'Saving…' : isEdit ? 'Save changes' : 'Create project'}
          </button>
        </div>
      </form>
    </div>
  )
}
