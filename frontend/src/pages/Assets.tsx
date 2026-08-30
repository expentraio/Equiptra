import { useEffect, useMemo, useState } from 'react'
import { api } from '../lib/api'
import type { Asset } from '../types'
import { AssetTag } from '../components/AssetTag'
import { AssetDetailPanel } from '../components/AssetDetailPanel'
import { ProductThumbnail } from '../components/ProductThumbnail'
import { SearchIcon, ScanIcon } from '../components/icons'

const CATEGORIES = [
  'Sound', 'Vision', 'Cameras', 'Power', 'Cases', 'Networking', 'Card',
  'Grip', 'Lighting', 'Computers', 'Control', 'VTR', 'Other', 'Cable', 'Vehicle', 'Test',
]

const statusLabel: Record<Asset['status'], string> = {
  active: 'Available',
  written_off: 'Written off',
  sold: 'Sold',
  missing: 'Missing',
}

const statusBadgeClass: Record<Asset['status'], string> = {
  active: 'bg-teal-fill text-teal',
  written_off: 'bg-[#EDEBE4] text-ink-soft',
  sold: 'bg-[#EDEBE4] text-ink-soft',
  missing: 'bg-red-fill text-red',
}

export function Assets() {
  const [assets, setAssets] = useState<Asset[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState<string | null>(null)
  const [selected, setSelected] = useState<Asset | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    const params = new URLSearchParams()
    if (search) params.set('search', search)
    if (category) params.set('category', category)
    const timeout = setTimeout(() => {
      api
        .get<Asset[]>(`/assets?${params.toString()}`)
        .then(setAssets)
        .catch(() => {})
        .finally(() => setLoading(false))
    }, 200)
    return () => {
      controller.abort()
      clearTimeout(timeout)
    }
  }, [search, category])

  const subtitle = useMemo(() => {
    if (loading) return 'Loading…'
    return `${assets.length} item${assets.length === 1 ? '' : 's'}${category ? ` in ${category}` : ''} · LDMtv store`
  }, [assets.length, category, loading])

  return (
    <div>
      <div className="mb-5.5 flex flex-wrap items-start justify-between gap-5">
        <div>
          <h1 className="text-[21px] font-bold">Assets</h1>
          <p className="mt-1 text-[13px] text-ink-soft">{subtitle}</p>
        </div>
      </div>

      <div className="mb-4.5 flex gap-2">
        <div className="flex max-w-[420px] flex-1 items-center gap-2 rounded-control border border-border bg-surface px-3.5 py-2.25">
          <SearchIcon className="h-4 w-4 shrink-0 text-ink-soft" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, asset number, or serial…"
            className="w-full bg-transparent text-[13.5px] outline-none"
          />
        </div>
        <button
          className="flex items-center gap-1.5 rounded-control bg-ink px-4 py-2.25 text-[13px] font-medium text-white hover:opacity-88"
          title="Scan a barcode via your phone camera"
        >
          <ScanIcon className="h-4 w-4" />
          Scan tag
        </button>
      </div>

      <div className="mb-5 flex flex-wrap gap-2">
        <Chip active={category === null} onClick={() => setCategory(null)}>
          All
        </Chip>
        {CATEGORIES.map((c) => (
          <Chip key={c} active={category === c} onClick={() => setCategory(c)}>
            {c}
          </Chip>
        ))}
      </div>

      <div className="grid grid-cols-[repeat(auto-fill,minmax(230px,1fr))] gap-3">
        {assets.map((asset) => (
          <button
            key={asset.id}
            onClick={() => setSelected(asset)}
            className="rounded-card border border-border bg-surface p-3.5 px-4 text-left transition-colors hover:border-border-strong"
          >
            <div className="mb-2.5 flex items-start gap-3">
              <ProductThumbnail url={asset.product_image_url} size="card" />
              <div>
                <div className="mb-0.5 text-[14px] font-semibold leading-tight">{asset.product_name}</div>
                <div className="text-[12px] text-ink-soft">{asset.category}</div>
              </div>
            </div>
            <AssetTag number={asset.asset_number} />
            <div className="mt-3 flex items-center justify-between text-[12px] text-ink-soft">
              <span>{asset.location || '—'}</span>
              <span className={`rounded-full px-2.25 py-[3px] text-[11px] font-semibold ${statusBadgeClass[asset.status]}`}>
                {statusLabel[asset.status]}
              </span>
            </div>
          </button>
        ))}
      </div>

      {!loading && assets.length === 0 && (
        <div className="mt-10 text-center text-[13px] text-ink-soft">No assets match your search.</div>
      )}

      {selected && <AssetDetailPanel asset={selected} onClose={() => setSelected(null)} />}
    </div>
  )
}

function Chip({ active, onClick, children }: { active: boolean; onClick: () => void; children: string }) {
  return (
    <button
      onClick={onClick}
      className={`rounded-full border px-3.25 py-1.5 text-[12.5px] font-medium ${
        active ? 'border-ink bg-ink text-white' : 'border-border bg-surface text-ink-soft'
      }`}
    >
      {children}
    </button>
  )
}
