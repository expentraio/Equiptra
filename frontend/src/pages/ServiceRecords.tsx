import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import type { ServiceRecord, ServiceStatus } from '../types'
import { AssetTag } from '../components/AssetTag'

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

const FILTERS: { label: string; value: string }[] = [
  { label: 'All', value: '' },
  { label: 'Open', value: 'open' },
  { label: 'In progress', value: 'in_progress' },
  { label: 'Resolved', value: 'resolved' },
]

export function ServiceRecords() {
  const [records, setRecords] = useState<ServiceRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [status, setStatus] = useState('')
  const navigate = useNavigate()

  useEffect(() => {
    setLoading(true)
    const params = new URLSearchParams()
    if (status) params.set('status', status)
    api
      .get<ServiceRecord[]>(`/service-records?${params.toString()}`)
      .then(setRecords)
      .finally(() => setLoading(false))
  }, [status])

  return (
    <div>
      <div className="mb-5.5">
        <h1 className="text-[21px] font-bold">Services / Repairs</h1>
        <p className="mt-1 text-[13px] text-ink-soft">
          Fault reports logged automatically at check-in or via the public field-report form
        </p>
      </div>

      <div className="mb-4.5 flex flex-wrap gap-2">
        {FILTERS.map((f) => (
          <button
            key={f.value}
            onClick={() => setStatus(f.value)}
            className={`rounded-full border px-3.25 py-1.5 text-[12.5px] font-medium ${
              status === f.value ? 'border-ink bg-ink text-white' : 'border-border bg-surface text-ink-soft'
            }`}
          >
            {f.label}
          </button>
        ))}
      </div>

      <div className="overflow-hidden rounded-card border border-border bg-surface">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[680px] border-collapse">
            <thead>
              <tr>
                {['Asset', 'Fault', 'Source', 'Reporter', 'Date logged', 'Status'].map((h) => (
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
              {records.map((rec) => (
                <tr
                  key={rec.id}
                  onClick={() => navigate(`/services/${rec.id}`)}
                  className="cursor-pointer hover:bg-off-white"
                >
                  <td className="border-b border-border px-4 py-3.25 text-[13px]">
                    <div className="flex items-center gap-2">
                      <AssetTag number={rec.asset_number} />
                      <span className="text-ink-soft">{rec.product_name}</span>
                    </div>
                  </td>
                  <td className="max-w-[240px] truncate border-b border-border px-4 py-3.25 text-[13px]">
                    {rec.fault_description}
                  </td>
                  <td className="border-b border-border px-4 py-3.25 text-[13px] text-ink-soft">
                    {sourceLabel[rec.source] ?? rec.source}
                  </td>
                  <td className="border-b border-border px-4 py-3.25 text-[13px] text-ink-soft">
                    {rec.reporter_user_name ?? rec.reporter_name ?? '—'}
                  </td>
                  <td className="border-b border-border px-4 py-3.25 text-[13px]">{formatDate(rec.date_reported)}</td>
                  <td className="border-b border-border px-4 py-3.25 text-[13px]">
                    <span className={`rounded-full px-2.5 py-1 text-[11px] font-semibold ${statusPillClass[rec.status]}`}>
                      {statusLabel[rec.status]}
                    </span>
                  </td>
                </tr>
              ))}
              {!loading && records.length === 0 && (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-[13px] text-ink-soft">
                    No service records{status ? ` with status "${statusLabel[status as ServiceStatus]}"` : ''}.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
