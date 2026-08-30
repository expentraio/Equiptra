import { Fragment, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, ApiError } from '../lib/api'
import type { AllocationWithConflicts, BookingRequest, Project } from '../types'
import { AlertTriangleIcon, ChevronLeftIcon, FileIcon, PlusIcon } from '../components/icons'
import { NewBookingRequestModal } from '../components/NewBookingRequestModal'
import { AllocationPanel } from '../components/AllocationPanel'

function formatDate(d: string) {
  return new Date(d).toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })
}

function formatDateRange(start: string, end: string) {
  return `${formatDate(start)} – ${formatDate(end)}`
}

const projectStatusPillClass: Record<Project['status'], string> = {
  confirmed: 'bg-teal-fill text-teal',
  tentative: 'bg-[#EDEBE4] text-ink-soft',
  in_progress: 'bg-teal-fill text-teal',
  completed: 'bg-[#EDEBE4] text-ink-soft',
  cancelled: 'bg-red-fill text-red',
}

const statusPillClass: Record<BookingRequest['status'], string> = {
  draft: 'bg-[#EDEBE4] text-ink-soft',
  reserved: 'bg-[#EDEBE4] text-ink-soft',
  partially_allocated: 'bg-amber-fill text-amber',
  out: 'bg-teal-fill text-teal',
  returned: 'bg-[#EDEBE4] text-ink-soft',
  cancelled: 'bg-red-fill text-red',
}

export function BookingBoard() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [project, setProject] = useState<Project | null>(null)
  const [requests, setRequests] = useState<BookingRequest[]>([])
  const [loading, setLoading] = useState(true)
  const [showNew, setShowNew] = useState(false)
  const [expanded, setExpanded] = useState<number | null>(null)
  const [conflictCount, setConflictCount] = useState(0)
  const [actionError, setActionError] = useState<string | null>(null)

  function reload() {
    if (!id) return
    setLoading(true)
    Promise.all([api.get<Project>(`/projects/${id}`), api.get<BookingRequest[]>(`/booking-requests?project_id=${id}`)])
      .then(([p, reqs]) => {
        setProject(p)
        setRequests(reqs)
        return Promise.all(
          reqs
            .filter((r) => r.allocated_count > 0)
            .map((r) => api.get<AllocationWithConflicts[]>(`/booking-requests/${r.id}/allocations`)),
        )
      })
      .then((allocationLists) => {
        const count = allocationLists.flat().filter((a) => a.conflicts.length > 0).length
        setConflictCount(count)
      })
      .finally(() => setLoading(false))
  }

  useEffect(reload, [id])

  async function cancelRequest(reqId: number) {
    await api.post(`/booking-requests/${reqId}/cancel`)
    reload()
  }

  async function cancelProject() {
    if (!project) return
    if (!confirm(`Cancel "${project.name}"? Its status will be set to cancelled.`)) return
    setActionError(null)
    try {
      await api.post(`/projects/${project.id}/cancel`)
      reload()
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : 'Could not cancel project')
    }
  }

  async function deleteProject() {
    if (!project) return
    if (!confirm(`Permanently delete "${project.name}"? This cannot be undone.`)) return
    setActionError(null)
    try {
      await api.delete(`/projects/${project.id}`)
      navigate('/projects')
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : 'Could not delete project')
    }
  }

  if (loading && !project) {
    return <div className="text-[13px] text-ink-soft">Loading…</div>
  }
  if (!project) {
    return <div className="text-[13px] text-ink-soft">Project not found.</div>
  }

  return (
    <div>
      <div className="mb-5.5 flex flex-wrap items-start justify-between gap-5">
        <div>
          <div className="flex items-center gap-2.5">
            <h1 className="text-[21px] font-bold">{project.name}</h1>
            <span
              className={`rounded-full px-2.5 py-1 text-[11px] font-semibold ${projectStatusPillClass[project.status]}`}
            >
              {project.status.replace('_', ' ')}
            </span>
          </div>
          <p className="mt-1 text-[13px] text-ink-soft">
            {formatDateRange(project.start_date, project.end_date)}
            {project.client ? ` · Client: ${project.client}` : ''}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Link
            to={`/carnets?project=${project.id}`}
            className="flex items-center gap-1.5 rounded-full border border-border px-3.5 py-1.5 text-[12.5px] font-medium text-ink-soft hover:bg-off-white"
          >
            <FileIcon className="h-3.5 w-3.5" />
            Carnet
          </Link>
          <Link
            to={`/delivery-notes?project=${project.id}`}
            className="flex items-center gap-1.5 rounded-full border border-border px-3.5 py-1.5 text-[12.5px] font-medium text-ink-soft hover:bg-off-white"
          >
            <FileIcon className="h-3.5 w-3.5" />
            Delivery note
          </Link>
          <Link
            to="/projects"
            className="flex items-center gap-1.5 rounded-full border border-border px-3.5 py-1.5 text-[12.5px] font-medium text-ink-soft hover:bg-off-white"
          >
            <ChevronLeftIcon className="h-3.5 w-3.5" />
            Back
          </Link>
          <button
            onClick={() => setShowNew(true)}
            className="flex items-center gap-1.5 rounded-control bg-teal px-4 py-2.25 text-[13px] font-medium text-white hover:opacity-90"
          >
            <PlusIcon className="h-4 w-4" />
            New request
          </button>
        </div>
      </div>

      <div className="mb-5.5 flex flex-wrap items-center gap-4 border-t border-border pt-3.5">
        <span className="text-[11px] font-semibold uppercase tracking-[.05em] text-ink-soft">Project actions</span>
        {project.status !== 'cancelled' && (
          <button onClick={cancelProject} className="text-[12.5px] font-medium text-ink-soft hover:text-red">
            Cancel project
          </button>
        )}
        <button
          onClick={deleteProject}
          disabled={requests.length > 0}
          title={requests.length > 0 ? 'Cannot delete a project with booking requests attached — use Cancel instead' : undefined}
          className="text-[12.5px] font-medium text-ink-soft hover:text-red disabled:cursor-not-allowed disabled:text-[#C9C5BA] disabled:hover:text-[#C9C5BA]"
        >
          Delete project
        </button>
      </div>

      {actionError && (
        <div className="mb-4.5 rounded-control border border-red-fill bg-red-fill px-3.5 py-2.5 text-[13px] font-medium text-red">
          {actionError}
        </div>
      )}

      {conflictCount > 0 && (
        <div className="mb-4.5 flex items-start gap-2.5 rounded-control border border-[#E8C5BE] bg-red-fill px-4 py-3 text-[13px] font-medium text-red">
          <AlertTriangleIcon className="mt-0.5 h-4 w-4 shrink-0" />
          <span>
            {conflictCount} allocation{conflictCount === 1 ? '' : 's'} on this project clash with a booking elsewhere. Expand a
            reservation below to see details and resolve.
          </span>
        </div>
      )}

      <div className="overflow-hidden rounded-card border border-border bg-surface">
        <div className="overflow-x-auto">
        <table className="w-full min-w-[560px] border-collapse">
          <thead>
            <tr>
              {['Item', 'Qty', 'Dates', 'Status', ''].map((h) => (
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
            {requests.map((req) => (
              <Fragment key={req.id}>
                <tr
                  onClick={() => setExpanded(expanded === req.id ? null : req.id)}
                  className="cursor-pointer hover:bg-off-white"
                >
                  <td className="border-b border-border px-4 py-3.25 text-[13px]">
                    <div className="font-medium">{req.product_name ?? req.placeholder_description}</div>
                    {req.shortage_flag && (
                      <span className="mt-1 inline-flex items-center gap-1 rounded-full bg-amber-fill px-2 py-[2px] text-[10.5px] font-semibold text-amber">
                        Short
                      </span>
                    )}
                  </td>
                  <td className="border-b border-border px-4 py-3.25 text-[13px]">
                    {req.allocated_count}/{req.quantity_requested}
                  </td>
                  <td className="border-b border-border px-4 py-3.25 text-[13px]">{formatDateRange(req.date_out, req.date_in)}</td>
                  <td className="border-b border-border px-4 py-3.25 text-[13px]">
                    <span className={`rounded-full px-2.5 py-1 text-[11px] font-semibold ${statusPillClass[req.status]}`}>
                      {req.status.replace('_', ' ')}
                    </span>
                  </td>
                  <td className="border-b border-border px-4 py-3.25 text-right text-[12px]">
                    {req.status !== 'cancelled' && req.status !== 'returned' && (
                      <button
                        onClick={(e) => {
                          e.stopPropagation()
                          void cancelRequest(req.id)
                        }}
                        className="font-medium text-ink-soft hover:text-red"
                      >
                        Cancel
                      </button>
                    )}
                  </td>
                </tr>
                {expanded === req.id && (
                  <tr>
                    <td colSpan={5} className="p-0">
                      <AllocationPanel request={req} onChanged={reload} />
                    </td>
                  </tr>
                )}
              </Fragment>
            ))}
            {requests.length === 0 && (
              <tr>
                <td colSpan={5} className="px-4 py-8 text-center text-[13px] text-ink-soft">
                  Nothing reserved for this project yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
        </div>
      </div>

      {showNew && (
        <NewBookingRequestModal
          project={project}
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
