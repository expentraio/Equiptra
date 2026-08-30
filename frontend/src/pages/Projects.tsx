import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import type { Project } from '../types'
import { FileIcon, PlusIcon } from '../components/icons'
import { ProjectFormModal } from '../components/ProjectFormModal'

const statusPillClass: Record<Project['status'], string> = {
  confirmed: 'bg-teal-fill text-teal',
  tentative: 'bg-[#EDEBE4] text-ink-soft',
  in_progress: 'bg-teal-fill text-teal',
  completed: 'bg-[#EDEBE4] text-ink-soft',
  cancelled: 'bg-red-fill text-red',
}

function formatDateRange(start: string, end: string) {
  const s = new Date(start)
  const e = new Date(end)
  const sameMonth = s.getMonth() === e.getMonth() && s.getFullYear() === e.getFullYear()
  const opts: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'short' }
  const startStr = s.toLocaleDateString('en-GB', sameMonth ? { day: 'numeric' } : opts)
  const endStr = e.toLocaleDateString('en-GB', { ...opts, year: 'numeric' })
  return `${startStr}–${endStr}`
}

export function Projects() {
  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(true)
  const [showNew, setShowNew] = useState(false)
  const navigate = useNavigate()

  function reload() {
    setLoading(true)
    api
      .get<Project[]>('/projects')
      .then(setProjects)
      .finally(() => setLoading(false))
  }

  useEffect(reload, [])

  return (
    <div>
      <div className="mb-5.5 flex flex-wrap items-start justify-between gap-5">
        <div>
          <h1 className="text-[21px] font-bold">Projects</h1>
          <p className="mt-1 text-[13px] text-ink-soft">Booking status and asset assignment</p>
        </div>
        <button
          onClick={() => setShowNew(true)}
          className="flex items-center gap-1.5 rounded-control bg-teal px-4 py-2.25 text-[13px] font-medium text-white hover:opacity-90"
        >
          <PlusIcon className="h-4 w-4" />
          New project
        </button>
      </div>

      <div className="flex flex-col gap-2.5">
        {projects.map((p) => (
          <div
            key={p.id}
            onClick={() => navigate(`/projects/${p.id}`)}
            className="flex cursor-pointer items-center justify-between rounded-card border border-border bg-surface px-4.5 py-4 hover:border-border-strong"
          >
            <div>
              <div className="mb-0.75 text-[15px] font-bold">{p.name}</div>
              <div className="text-[12.5px] text-ink-soft">{p.client || '—'}</div>
            </div>
            <div className="flex items-center gap-3.5">
              <div className="text-right text-[12.5px] text-ink-soft">{formatDateRange(p.start_date, p.end_date)}</div>
              <span className={`rounded-full px-2.75 py-1 text-[11.5px] font-semibold ${statusPillClass[p.status]}`}>
                {p.status.replace('_', ' ')}
              </span>
              <button
                onClick={(e) => {
                  e.stopPropagation()
                  navigate(`/carnets?project=${p.id}`)
                }}
                title="Generate carnet"
                className="flex rounded-lg border border-border bg-surface p-1.5 text-ink-soft hover:border-teal hover:text-teal"
              >
                <FileIcon className="h-3.75 w-3.75" />
              </button>
            </div>
          </div>
        ))}
        {!loading && projects.length === 0 && (
          <div className="mt-10 text-center text-[13px] text-ink-soft">No projects yet — create one to get started.</div>
        )}
      </div>

      {showNew && (
        <ProjectFormModal
          onClose={() => setShowNew(false)}
          onSaved={() => {
            setShowNew(false)
            reload()
          }}
        />
      )}
    </div>
  )
}
