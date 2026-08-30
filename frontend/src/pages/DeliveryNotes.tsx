import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '../lib/api'
import type { DeliveryNoteView, Project } from '../types'
import { DownloadIcon } from '../components/icons'

export function DeliveryNotes() {
  const [searchParams, setSearchParams] = useSearchParams()
  const projectId = searchParams.get('project')
  const [projects, setProjects] = useState<Project[]>([])

  useEffect(() => {
    api.get<Project[]>('/projects').then(setProjects)
  }, [])

  return (
    <div>
      <div className="mb-5.5">
        <h1 className="text-[21px] font-bold">Delivery notes</h1>
        <p className="mt-1 text-[13px] text-ink-soft">LDMtv-branded paperwork for what's going out on a project</p>
      </div>

      {!projectId && (
        <div className="flex flex-col gap-2">
          {projects.map((p) => (
            <button
              key={p.id}
              onClick={() => setSearchParams({ project: String(p.id) })}
              className="flex items-center justify-between rounded-card border border-border bg-surface px-4 py-3 text-left hover:border-border-strong"
            >
              <span className="text-[13.5px] font-medium">{p.name}</span>
              <span className="text-[12px] text-ink-soft">{p.client}</span>
            </button>
          ))}
          {projects.length === 0 && <div className="text-[13px] text-ink-soft">No projects yet.</div>}
        </div>
      )}

      {projectId && <DeliveryNoteDetail projectId={Number(projectId)} onBack={() => setSearchParams({})} />}
    </div>
  )
}

function DeliveryNoteDetail({ projectId, onBack }: { projectId: number; onBack: () => void }) {
  const [view, setView] = useState<DeliveryNoteView | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    setLoading(true)
    api
      .get<DeliveryNoteView>(`/projects/${projectId}/delivery-note`)
      .then(setView)
      .finally(() => setLoading(false))
  }, [projectId])

  if (loading) return <div className="text-[13px] text-ink-soft">Loading…</div>
  if (!view) return <div className="text-[13px] text-ink-soft">Could not load delivery note.</div>

  return (
    <div>
      <button onClick={onBack} className="mb-4 text-[12.5px] font-medium text-ink-soft hover:text-ink">
        ← All projects
      </button>

      <div className="mb-4 flex items-center justify-between rounded-card border border-border bg-surface px-5 py-4.5">
        <div>
          <div className="text-[15px] font-bold">{view.project.name}</div>
          <p className="mt-0.75 text-[12.5px] text-ink-soft">
            {view.lines.length} item{view.lines.length === 1 ? '' : 's'} · total weight {view.total_weight_kg.toFixed(1)} kg · total
            value £{view.total_value.toLocaleString()}
          </p>
        </div>
        <a
          href={`/api/projects/${projectId}/delivery-note/export.pdf`}
          className="flex items-center gap-1.5 rounded-control bg-teal px-4.5 py-2.5 text-[13px] font-medium text-white hover:opacity-90"
        >
          <DownloadIcon className="h-4 w-4" />
          Export PDF
        </a>
      </div>

      <div className="overflow-hidden rounded-card border border-border bg-surface">
        <div className="overflow-x-auto">
        <table className="w-full min-w-[520px] border-collapse">
          <thead>
            <tr>
              {['Item', 'Qty', 'Asset Number', 'Serial Number'].map((h) => (
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
            {view.lines.map((line, i) => (
              <tr key={i}>
                <td className={`border-b border-border px-4 py-3.25 text-[13px] ${line.is_accessory ? 'pl-8 italic text-ink-soft' : ''}`}>
                  {line.description}
                  {line.is_accessory ? ' (accessory)' : ''}
                </td>
                <td className="border-b border-border px-4 py-3.25 text-[13px]">{line.quantity}</td>
                <td className="border-b border-border px-4 py-3.25 text-[13px]">{line.asset_number ?? ''}</td>
                <td className="border-b border-border px-4 py-3.25 text-[13px]">{line.serial_number ?? ''}</td>
              </tr>
            ))}
            {view.lines.length === 0 && (
              <tr>
                <td colSpan={4} className="px-4 py-8 text-center text-[13px] text-ink-soft">
                  No allocated assets on this project yet.
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
