import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '../lib/api'
import type { CarnetView, Project } from '../types'
import { AlertTriangleIcon, DownloadIcon } from '../components/icons'

export function Carnets() {
  const [searchParams, setSearchParams] = useSearchParams()
  const projectId = searchParams.get('project')
  const [projects, setProjects] = useState<Project[]>([])

  useEffect(() => {
    api.get<Project[]>('/projects').then(setProjects)
  }, [])

  const needingCarnet = projects.filter((p) => p.carnet_required && p.status === 'confirmed')

  return (
    <div>
      <div className="mb-5.5">
        <h1 className="text-[21px] font-bold">Carnets</h1>
        <p className="mt-1 text-[13px] text-ink-soft">Any project's confirmed bookings can be exported — not every job needs one</p>
      </div>

      {!projectId && needingCarnet.length > 0 && (
        <div className="mb-5.5">
          <div className="mb-2.5 text-[11px] font-semibold uppercase tracking-[.06em] text-ink-soft">
            Confirmed projects still needing a carnet
          </div>
          <div className="flex flex-col gap-2">
            {needingCarnet.map((p) => (
              <button
                key={p.id}
                onClick={() => setSearchParams({ project: String(p.id) })}
                className="flex items-center justify-between rounded-card border border-amber-fill bg-amber-fill px-4 py-3 text-left"
              >
                <span className="text-[13.5px] font-medium text-amber">{p.name}</span>
                <span className="text-[12px] text-amber">Generate →</span>
              </button>
            ))}
          </div>
        </div>
      )}

      {!projectId && (
        <div>
          <div className="mb-2.5 text-[11px] font-semibold uppercase tracking-[.06em] text-ink-soft">All projects</div>
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
          </div>
        </div>
      )}

      {projectId && <CarnetDetail projectId={Number(projectId)} onBack={() => setSearchParams({})} />}
    </div>
  )
}

function CarnetDetail({ projectId, onBack }: { projectId: number; onBack: () => void }) {
  const [view, setView] = useState<CarnetView | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    setLoading(true)
    api
      .get<CarnetView>(`/projects/${projectId}/carnet`)
      .then(setView)
      .finally(() => setLoading(false))
  }, [projectId])

  if (loading) return <div className="text-[13px] text-ink-soft">Loading…</div>
  if (!view) return <div className="text-[13px] text-ink-soft">Could not load carnet.</div>

  return (
    <div>
      <button onClick={onBack} className="mb-4 text-[12.5px] font-medium text-ink-soft hover:text-ink">
        ← All projects
      </button>

      <div className="mb-4 flex items-center justify-between rounded-card border border-border bg-surface px-5 py-4.5">
        <div>
          <div className="text-[15px] font-bold">{view.project_name}</div>
          <p className="mt-0.75 text-[12.5px] text-ink-soft">
            {view.lines.length} item{view.lines.length === 1 ? '' : 's'} · total weight {view.total_weight_kg.toFixed(1)} kg · total
            value £{view.total_value.toLocaleString()}
          </p>
        </div>
        <div className="flex gap-2">
          <a
            href={`/api/projects/${projectId}/carnet/export.csv`}
            className="flex items-center gap-1.5 rounded-control border border-border-strong bg-surface px-4.5 py-2.5 text-[13px] font-medium text-teal"
          >
            <DownloadIcon className="h-4 w-4" />
            Export CSV
          </a>
          <a
            href={`/api/projects/${projectId}/carnet/export.pdf`}
            aria-disabled={view.missing_origin}
            onClick={(e) => view.missing_origin && e.preventDefault()}
            className={`flex items-center gap-1.5 rounded-control px-4.5 py-2.5 text-[13px] font-medium text-white ${
              view.missing_origin ? 'cursor-not-allowed bg-ink-soft' : 'bg-teal hover:opacity-90'
            }`}
          >
            <DownloadIcon className="h-4 w-4" />
            Export PDF
          </a>
        </div>
      </div>

      {view.missing_origin && (
        <div className="mb-4 flex items-start gap-2.5 rounded-control border border-[#E8C5BE] bg-red-fill px-4 py-3 text-[13px] font-medium text-red">
          <AlertTriangleIcon className="mt-0.5 h-4 w-4 shrink-0" />
          <span>
            One or more items are missing a country of origin. PDF export is blocked until this is fixed in Products — CSV export
            still works with blank origin cells.
          </span>
        </div>
      )}

      <div className="overflow-hidden rounded-card border border-border bg-surface">
        <table className="w-full border-collapse">
          <thead>
            <tr>
              {['Description', 'Qty', 'Weight', 'Value', 'Origin'].map((h) => (
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
            {view.lines.map((line) => (
              <tr key={line.product_id} className={line.missing_origin ? 'bg-red-fill' : ''}>
                <td className="border-b border-border px-4 py-3.25 text-[13px]">{line.description}</td>
                <td className="border-b border-border px-4 py-3.25 text-[13px]">{line.quantity}</td>
                <td className="border-b border-border px-4 py-3.25 text-[13px]">{line.total_weight_kg.toFixed(1)} kg</td>
                <td className="border-b border-border px-4 py-3.25 text-[13px]">£{line.total_value.toLocaleString()}</td>
                <td className={`border-b border-border px-4 py-3.25 text-[13px] ${line.missing_origin ? 'font-semibold text-red' : ''}`}>
                  {line.country_of_origin_code || 'Missing'}
                </td>
              </tr>
            ))}
            {view.lines.length === 0 && (
              <tr>
                <td colSpan={5} className="px-4 py-8 text-center text-[13px] text-ink-soft">
                  No allocated assets on this project yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
