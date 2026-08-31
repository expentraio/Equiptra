import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, ApiError } from '../lib/api'
import type { BookingAllocation, ServiceRecord, ServiceStatus } from '../types'
import { AssetTag } from '../components/AssetTag'
import { ChevronLeftIcon } from '../components/icons'

interface HistoryAllocation extends BookingAllocation {
  project_name: string
  date_out: string
  date_in: string
}

function formatDate(d: string) {
  return new Date(d).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}

const statusLabel: Record<ServiceStatus, string> = {
  open: 'Open',
  in_progress: 'In progress',
  under_investigation: 'Under investigation',
  resolved: 'Resolved',
}

const statusPillClass: Record<ServiceStatus, string> = {
  open: 'bg-red-fill text-red',
  in_progress: 'bg-amber-fill text-amber',
  under_investigation: 'bg-amber-fill text-amber',
  resolved: 'bg-teal-fill text-teal',
}

const sourceLabel: Record<string, string> = {
  checkin_damage: 'Check-in damage',
  field_report: 'Field report',
  monday_report: 'Monday.com (legacy)',
}

export function ServiceRecordDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [record, setRecord] = useState<ServiceRecord | null>(null)
  const [history, setHistory] = useState<HistoryAllocation[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [updating, setUpdating] = useState(false)

  function reload() {
    if (!id) return
    setLoading(true)
    api
      .get<ServiceRecord>(`/service-records/${id}`)
      .then((rec) => {
        setRecord(rec)
        return api.get<{ allocations: HistoryAllocation[] }>(`/assets/${rec.asset_id}/history`)
      })
      .then((data) => setHistory(data.allocations))
      .finally(() => setLoading(false))
  }

  useEffect(reload, [id])

  async function setStatus(status: ServiceStatus) {
    if (!record) return
    setUpdating(true)
    setError(null)
    try {
      await api.put(`/service-records/${record.id}`, {
        asset_id: record.asset_id,
        date_reported: record.date_reported,
        fault_description: record.fault_description,
        status,
        monday_item_id: record.monday_item_id ?? null,
        resolved_date: record.resolved_date ?? null,
        resolution_notes: record.resolution_notes ?? null,
      })
      reload()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not update status')
    } finally {
      setUpdating(false)
    }
  }

  if (loading && !record) {
    return <div className="text-[13px] text-ink-soft">Loading…</div>
  }
  if (!record) {
    return <div className="text-[13px] text-ink-soft">Service record not found.</div>
  }

  return (
    <div>
      <button
        onClick={() => navigate('/services')}
        className="mb-4 inline-flex items-center gap-1.5 text-[12.5px] font-medium text-ink-soft hover:text-ink"
      >
        <ChevronLeftIcon className="h-3.5 w-3.5" />
        Back to Services
      </button>

      <div className="mb-5.5 flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-2.5">
            <AssetTag number={record.asset_number} />
            <h1 className="text-[18px] font-bold">{record.product_name}</h1>
            <span className={`rounded-full px-2.5 py-1 text-[11px] font-semibold ${statusPillClass[record.status]}`}>
              {statusLabel[record.status]}
            </span>
          </div>
          <p className="mt-1.5 text-[13px] text-ink-soft">
            Reported {formatDate(record.date_reported)} · {sourceLabel[record.source] ?? record.source} ·{' '}
            {record.reporter_user_name ?? record.reporter_name ?? 'Unknown reporter'}
            {record.reporter_email ? ` (${record.reporter_email})` : ''}
          </p>
        </div>
        <Link
          to={`/products/${record.product_id}?highlight=${record.asset_id}`}
          className="text-[12.5px] font-medium text-teal hover:underline"
        >
          View asset →
        </Link>
      </div>

      <div className="mb-5 rounded-card border border-border bg-surface p-4.5">
        <div className="mb-1 text-[11px] font-semibold uppercase tracking-[.05em] text-ink-soft">Fault description</div>
        <p className="text-[13.5px]">{record.fault_description}</p>
        {record.resolution_notes && (
          <>
            <div className="mt-3 mb-1 text-[11px] font-semibold uppercase tracking-[.05em] text-ink-soft">Resolution notes</div>
            <p className="text-[13.5px]">{record.resolution_notes}</p>
          </>
        )}
        {record.status === 'resolved' && record.resolved_date && (
          <p className="mt-3 text-[12px] text-ink-soft">
            Resolved {formatDate(record.resolved_date)}
            {record.resolved_by_name ? ` by ${record.resolved_by_name}` : ''}
          </p>
        )}
      </div>

      <div className="mb-5 flex flex-wrap items-center gap-3 border-t border-border pt-4">
        <span className="text-[11px] font-semibold uppercase tracking-[.05em] text-ink-soft">Update status</span>
        {record.status !== 'in_progress' && record.status !== 'resolved' && (
          <button
            onClick={() => setStatus('in_progress')}
            disabled={updating}
            className="rounded-control bg-amber-fill px-3.5 py-1.5 text-[12.5px] font-medium text-amber hover:opacity-80 disabled:opacity-50"
          >
            Start work
          </button>
        )}
        {record.status !== 'resolved' && (
          <button
            onClick={() => setStatus('resolved')}
            disabled={updating}
            className="rounded-control bg-teal px-3.5 py-1.5 text-[12.5px] font-medium text-white hover:opacity-90 disabled:opacity-50"
          >
            Mark resolved
          </button>
        )}
        {record.status === 'resolved' && (
          <button
            onClick={() => setStatus('open')}
            disabled={updating}
            className="rounded-control border border-border px-3.5 py-1.5 text-[12.5px] font-medium text-ink-soft hover:bg-off-white disabled:opacity-50"
          >
            Reopen
          </button>
        )}
        {error && <span className="text-[12.5px] font-medium text-red">{error}</span>}
      </div>

      <div className="mb-3 text-[11px] font-semibold uppercase tracking-[.05em] text-ink-soft">
        Booking history for this asset
      </div>
      <div className="flex flex-col gap-2">
        {history.map((h) => (
          <div key={h.id} className="rounded-control border border-border bg-surface px-3.5 py-3 text-[12.5px]">
            <div className="flex items-center justify-between">
              <span className="font-medium">{h.project_name}</span>
              <span className="text-ink-soft">{h.status}</span>
            </div>
            <div className="mt-0.5 text-ink-soft">
              {formatDate(h.date_out)} – {formatDate(h.date_in)}
            </div>
          </div>
        ))}
        {history.length === 0 && <div className="text-[12.5px] text-ink-soft">No booking history for this asset yet.</div>}
      </div>
    </div>
  )
}
