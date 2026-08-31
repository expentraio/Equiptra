import { useEffect, useState } from 'react'
import { api, ApiError } from '../lib/api'
import type { AllocationWithConflicts, Asset, BookingRequest } from '../types'
import { AssetTag } from './AssetTag'
import { AlertTriangleIcon, CloseIcon, PlusIcon } from './icons'

function formatDate(d: string) {
  return new Date(d).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}

const statusLabel: Record<AllocationWithConflicts['status'], string> = {
  allocated: 'Allocated',
  checked_out: 'Checked out',
  returned: 'Returned',
}

const statusClass: Record<AllocationWithConflicts['status'], string> = {
  allocated: 'bg-[#EDEBE4] text-ink-soft',
  checked_out: 'bg-teal-fill text-teal',
  returned: 'bg-[#EDEBE4] text-ink-soft',
}

export function AllocationPanel({ request, onChanged }: { request: BookingRequest; onChanged: () => void }) {
  const [allocations, setAllocations] = useState<AllocationWithConflicts[]>([])
  const [loading, setLoading] = useState(true)
  const [showAdd, setShowAdd] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function reload() {
    setLoading(true)
    api
      .get<AllocationWithConflicts[]>(`/booking-requests/${request.id}/allocations`)
      .then(setAllocations)
      .finally(() => setLoading(false))
  }

  useEffect(reload, [request.id])

  function handleChanged() {
    reload()
    onChanged()
  }

  if (!request.product_id) {
    return (
      <div className="border-t border-border bg-off-white px-5 py-4 text-[13px] text-ink-soft">
        Pin a real product to this request before allocating specific assets.
      </div>
    )
  }

  return (
    <div className="border-t border-border bg-off-white px-5 py-4">
      {request.sub_hire_notes && (
        <div className="mb-3 rounded-control bg-amber-fill px-3.5 py-2.5 text-[12.5px] text-amber">
          <strong>Sub-hire notes:</strong> {request.sub_hire_notes}
        </div>
      )}

      {loading ? (
        <div className="py-2 text-[12.5px] text-ink-soft">Loading…</div>
      ) : (
        <div className="flex flex-col gap-2">
          {allocations.map((a) => (
            <AllocationRow key={a.id} allocation={a} onChanged={handleChanged} />
          ))}
          {allocations.length === 0 && <div className="py-1 text-[12.5px] text-ink-soft">No assets picked yet.</div>}
        </div>
      )}

      {error && <div className="mt-2 text-[12.5px] font-medium text-red">{error}</div>}

      {allocations.length < request.quantity_requested && (
        <button
          onClick={() => setShowAdd(true)}
          className="mt-3 flex items-center gap-1.5 rounded-control border border-border-strong bg-surface px-3.5 py-2 text-[12.5px] font-medium text-teal hover:border-teal"
        >
          <PlusIcon className="h-3.5 w-3.5" />
          Allocate an asset
        </button>
      )}

      {showAdd && (
        <AddAllocationForm
          request={request}
          onClose={() => setShowAdd(false)}
          onAdded={() => {
            setShowAdd(false)
            handleChanged()
          }}
          onError={setError}
        />
      )}
    </div>
  )
}

function AllocationRow({ allocation, onChanged }: { allocation: AllocationWithConflicts; onChanged: () => void }) {
  const [showCheckout, setShowCheckout] = useState(false)
  const [showCheckin, setShowCheckin] = useState(false)
  const [busy, setBusy] = useState(false)

  async function remove() {
    setBusy(true)
    try {
      await api.delete(`/booking-allocations/${allocation.id}`)
      onChanged()
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className={`rounded-control border px-3.5 py-3 ${allocation.conflicts.length > 0 ? 'border-[#E8C5BE] bg-red-fill' : 'border-border bg-surface'}`}>
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <AssetTag number={allocation.asset_number} />
          {allocation.serial_number && <span className="text-[12px] text-ink-soft">{allocation.serial_number}</span>}
        </div>
        <div className="flex items-center gap-2">
          <span className={`rounded-full px-2.5 py-[3px] text-[11px] font-semibold ${statusClass[allocation.status]}`}>
            {statusLabel[allocation.status]}
          </span>
          {allocation.status === 'allocated' && (
            <>
              <button
                onClick={() => setShowCheckout(true)}
                className="rounded-control bg-teal px-3 py-1.5 text-[12px] font-medium text-white hover:opacity-90"
              >
                Check out
              </button>
              <button
                onClick={remove}
                disabled={busy}
                className="rounded-control border border-border px-3 py-1.5 text-[12px] font-medium text-ink-soft hover:bg-off-white"
              >
                Remove
              </button>
            </>
          )}
          {allocation.status === 'checked_out' && (
            <button
              onClick={() => setShowCheckin(true)}
              className="rounded-control bg-ink px-3 py-1.5 text-[12px] font-medium text-white hover:opacity-90"
            >
              Check in
            </button>
          )}
        </div>
      </div>

      {allocation.conflicts.length > 0 && (
        <div className="mt-2 flex items-start gap-2 text-[12px] font-medium text-red">
          <AlertTriangleIcon className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <span>
            Already {allocation.conflicts[0].status} on "{allocation.conflicts[0].project_name}" for{' '}
            {formatDate(allocation.conflicts[0].date_out)} – {formatDate(allocation.conflicts[0].date_in)}.
          </span>
        </div>
      )}

      {allocation.status === 'checked_out' && (
        <div className="mt-2 text-[11.5px] text-ink-soft">
          Out since {allocation.checked_out_at && formatDate(allocation.checked_out_at)}
          {allocation.checked_out_by_name ? ` · ${allocation.checked_out_by_name}` : ''}
        </div>
      )}

      {showCheckout && (
        <CheckoutForm allocationId={allocation.id} onClose={() => setShowCheckout(false)} onDone={() => { setShowCheckout(false); onChanged() }} />
      )}
      {showCheckin && (
        <CheckinForm allocationId={allocation.id} onClose={() => setShowCheckin(false)} onDone={() => { setShowCheckin(false); onChanged() }} />
      )}
    </div>
  )
}

function CheckoutForm({ allocationId, onClose, onDone }: { allocationId: number; onClose: () => void; onDone: () => void }) {
  const [inspectionPassed, setInspectionPassed] = useState(false)
  const [notes, setNotes] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function submit() {
    setSubmitting(true)
    setError(null)
    try {
      await api.post(`/booking-allocations/${allocationId}/checkout`, {
        inspection_passed: inspectionPassed,
        condition_out_notes: notes || null,
      })
      onDone()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Checkout failed')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="mt-3 rounded-control border border-border bg-off-white p-3">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-[12.5px] font-semibold">Pre-release inspection</span>
        <button onClick={onClose} className="text-ink-soft hover:text-ink">
          <CloseIcon className="h-3.5 w-3.5" />
        </button>
      </div>
      <label className="mb-2 flex items-center gap-2 text-[12.5px] font-medium">
        <input type="checkbox" checked={inspectionPassed} onChange={(e) => setInspectionPassed(e.target.checked)} />
        Inspection passed — safe and functional to send out
      </label>
      <textarea
        value={notes}
        onChange={(e) => setNotes(e.target.value)}
        placeholder="Condition notes (optional)…"
        rows={2}
        className="mb-2 w-full rounded-control border border-border px-3 py-2 text-[12.5px] outline-none focus:border-teal"
      />
      {!inspectionPassed && (
        <div className="mb-2 text-[11.5px] text-amber">Checkout is blocked until inspection is marked passed.</div>
      )}
      {error && <div className="mb-2 text-[11.5px] font-medium text-red">{error}</div>}
      <button
        onClick={submit}
        disabled={!inspectionPassed || submitting}
        className="rounded-control bg-teal px-3.5 py-2 text-[12.5px] font-medium text-white hover:opacity-90 disabled:opacity-50"
      >
        {submitting ? 'Checking out…' : 'Confirm check out'}
      </button>
    </div>
  )
}

function CheckinForm({ allocationId, onClose, onDone }: { allocationId: number; onClose: () => void; onDone: () => void }) {
  const [notes, setNotes] = useState('')
  const [damageFlag, setDamageFlag] = useState(false)
  const [faultDescription, setFaultDescription] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function submit() {
    setSubmitting(true)
    setError(null)
    try {
      await api.post(`/booking-allocations/${allocationId}/checkin`, {
        condition_in_notes: notes || null,
        damage_flag: damageFlag,
        fault_description: damageFlag ? faultDescription || null : null,
      })
      onDone()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Check-in failed')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="mt-3 rounded-control border border-border bg-off-white p-3">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-[12.5px] font-semibold">Check in</span>
        <button onClick={onClose} className="text-ink-soft hover:text-ink">
          <CloseIcon className="h-3.5 w-3.5" />
        </button>
      </div>
      <textarea
        value={notes}
        onChange={(e) => setNotes(e.target.value)}
        placeholder="Condition notes…"
        rows={2}
        className="mb-2 w-full rounded-control border border-border px-3 py-2 text-[12.5px] outline-none focus:border-teal"
      />
      <label className="mb-2 flex items-center gap-2 text-[12.5px] font-medium text-red">
        <input type="checkbox" checked={damageFlag} onChange={(e) => setDamageFlag(e.target.checked)} />
        Damaged — log a service record
      </label>
      {damageFlag && (
        <textarea
          value={faultDescription}
          onChange={(e) => setFaultDescription(e.target.value)}
          placeholder="What's wrong with it?"
          rows={2}
          className="mb-2 w-full rounded-control border border-border px-3 py-2 text-[12.5px] outline-none focus:border-teal"
        />
      )}
      {error && <div className="mb-2 text-[11.5px] font-medium text-red">{error}</div>}
      <button
        onClick={submit}
        disabled={submitting}
        className="rounded-control bg-ink px-3.5 py-2 text-[12.5px] font-medium text-white hover:opacity-90 disabled:opacity-50"
      >
        {submitting ? 'Checking in…' : 'Confirm check in'}
      </button>
    </div>
  )
}

function AddAllocationForm({
  request,
  onClose,
  onAdded,
  onError,
}: {
  request: BookingRequest
  onClose: () => void
  onAdded: () => void
  onError: (msg: string | null) => void
}) {
  const [assets, setAssets] = useState<Asset[]>([])
  const [selected, setSelected] = useState<Asset | null>(null)
  const [warning, setWarning] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    api.get<Asset[]>(`/assets?product_id=${request.product_id}&status=active`).then(setAssets)
  }, [request.product_id])

  async function submit(override: boolean) {
    if (!selected) return
    setSubmitting(true)
    onError(null)
    setWarning(null)
    try {
      await api.post(`/booking-requests/${request.id}/allocations`, { asset_id: selected.id, override })
      onAdded()
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        const body = err.body as { message?: string; error?: string; conflicts?: { project_name: string }[] }
        if (body.conflicts && body.conflicts.length > 0) {
          setWarning(`Already committed elsewhere — on "${body.conflicts[0].project_name}" for an overlapping date range.`)
        } else {
          setWarning(body.message || 'This would exceed available stock.')
        }
      } else {
        onError(err instanceof ApiError ? err.message : 'Could not allocate asset')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="mt-3 rounded-control border border-border bg-surface p-3">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-[12.5px] font-semibold">Pick a specific asset</span>
        <button onClick={onClose} className="text-ink-soft hover:text-ink">
          <CloseIcon className="h-3.5 w-3.5" />
        </button>
      </div>
      <div className="flex flex-wrap gap-1.5">
        {assets.map((a) => (
          <button
            key={a.id}
            disabled={a.has_open_fault}
            title={a.has_open_fault ? 'Excluded — this asset has an open or in-progress fault' : undefined}
            onClick={() => {
              setSelected(a)
              setWarning(null)
            }}
            className={`rounded-control border px-2.5 py-1.5 text-[12px] font-medium ${
              a.has_open_fault
                ? 'cursor-not-allowed border-border bg-red-fill text-red opacity-70'
                : selected?.id === a.id
                  ? 'border-ink bg-ink text-white'
                  : 'border-border text-ink-soft hover:border-border-strong'
            }`}
          >
            {a.is_bulk ? `bulk (${a.quantity} held)` : a.asset_number}
            {a.has_open_fault ? ' · faulted' : ''}
          </button>
        ))}
        {assets.length === 0 && <span className="text-[12px] text-ink-soft">No active assets found for this product.</span>}
      </div>

      {warning && (
        <div className="mt-2 flex flex-col gap-2 rounded-control border border-[#E8C5BE] bg-red-fill px-3 py-2.5 text-[12px] text-red">
          <span>{warning}</span>
          <button
            onClick={() => submit(true)}
            disabled={submitting}
            className="self-start rounded-control border border-red px-3 py-1 text-[11.5px] font-semibold text-red hover:bg-white"
          >
            Allocate anyway
          </button>
        </div>
      )}

      <button
        onClick={() => submit(false)}
        disabled={!selected || submitting}
        className="mt-2.5 rounded-control bg-teal px-3.5 py-2 text-[12.5px] font-medium text-white hover:opacity-90 disabled:opacity-50"
      >
        {submitting ? 'Allocating…' : 'Allocate'}
      </button>
    </div>
  )
}
