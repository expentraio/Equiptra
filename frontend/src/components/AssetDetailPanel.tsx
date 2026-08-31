import { useEffect, useRef, useState, type ReactNode } from 'react'
import { api, ApiError } from '../lib/api'
import { useAuth } from '../context/AuthContext'
import type { Asset, BookingAllocation, Product, ServiceRecord } from '../types'
import { AssetTag } from './AssetTag'
import { ProductThumbnail } from './ProductThumbnail'
import { CloseIcon, UploadIcon } from './icons'

const statusLabel: Record<Asset['status'], string> = {
  active: 'Active',
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

const allocationStatusLabel: Record<BookingAllocation['status'], string> = {
  allocated: 'Allocated',
  checked_out: 'Checked out',
  returned: 'Returned',
}

interface HistoryAllocation extends BookingAllocation {
  project_name: string
  date_out: string
  date_in: string
}

function formatDate(d?: string) {
  if (!d) return ''
  return new Date(d).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}

export function AssetDetailPanel({ asset, onClose }: { asset: Asset; onClose: () => void }) {
  const { user } = useAuth()
  const [allocations, setAllocations] = useState<HistoryAllocation[]>([])
  const [serviceRecords, setServiceRecords] = useState<ServiceRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [photoUrl, setPhotoUrl] = useState(asset.product_image_url)
  const [uploading, setUploading] = useState(false)
  const [uploadError, setUploadError] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  async function handlePhotoSelected(file: File) {
    setUploading(true)
    setUploadError(null)
    try {
      const product = await api.uploadFile<Product>(`/products/${asset.product_id}/photo`, 'photo', file)
      setPhotoUrl(product.image_url)
    } catch (err) {
      setUploadError(err instanceof ApiError ? err.message : err instanceof Error ? err.message : 'Upload failed')
    } finally {
      setUploading(false)
    }
  }

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    api
      .get<{ allocations: HistoryAllocation[]; service_records: ServiceRecord[] }>(`/assets/${asset.id}/history`)
      .then((data) => {
        if (cancelled) return
        setAllocations(data.allocations)
        setServiceRecords(data.service_records)
      })
      .finally(() => !cancelled && setLoading(false))
    return () => {
      cancelled = true
    }
  }, [asset.id])

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-[rgba(15,23,42,.35)]" onClick={onClose}>
      <div
        className="h-full w-full max-w-[92vw] overflow-y-auto border-l border-border bg-surface px-6.5 pb-7.5 pt-6.5 sm:w-[380px]"
        onClick={(e) => e.stopPropagation()}
      >
        <button onClick={onClose} className="float-right p-1 text-ink-soft hover:text-ink">
          <CloseIcon className="h-4.5 w-4.5" />
        </button>
        <div className="mb-3.5 flex items-start gap-3.5">
          <ProductThumbnail url={photoUrl} size="panel" />
          <div className="flex-1">
            <h2 className="mb-0.5 text-[18px] font-bold">{asset.product_name}</h2>
            <div className="mb-2 text-[12.5px] text-ink-soft">
              {asset.category}
              {asset.serial_number ? ` · ${asset.serial_number}` : ''}
            </div>
            {user?.role === 'admin' && (
              <>
                <input
                  ref={fileInputRef}
                  type="file"
                  accept="image/jpeg,image/png,image/webp"
                  className="hidden"
                  onChange={(e) => {
                    const file = e.target.files?.[0]
                    if (file) void handlePhotoSelected(file)
                    e.target.value = ''
                  }}
                />
                <button
                  onClick={() => fileInputRef.current?.click()}
                  disabled={uploading}
                  className="flex items-center gap-1.5 rounded-control border border-border px-2.5 py-1.5 text-[11.5px] font-medium text-ink-soft hover:border-teal hover:text-teal disabled:opacity-60"
                >
                  <UploadIcon className="h-3.5 w-3.5" />
                  {uploading ? 'Uploading…' : photoUrl ? 'Replace photo' : 'Upload photo'}
                </button>
                {uploadError && <div className="mt-1 text-[11.5px] font-medium text-red">{uploadError}</div>}
              </>
            )}
          </div>
        </div>

        <div className="mb-1">
          <AssetTag number={asset.asset_number} />
        </div>

        <Row label="Serial number" value={asset.serial_number ?? '—'} />
        <Row label="Location" value={asset.location ?? '—'} />
        <Row
          label="Status"
          value={
            <span className={`rounded-full px-2.5 py-1 text-[11px] font-semibold ${statusBadgeClass[asset.status]}`}>
              {statusLabel[asset.status]}
            </span>
          }
        />
        {asset.is_bulk && <Row label="Quantity held" value={String(asset.quantity)} />}
        <Row label="Replacement value" value={asset.replacement_value != null ? `£${asset.replacement_value.toLocaleString()}` : '—'} />
        <Row label="Purchase price" value={asset.purchase_price != null ? `£${asset.purchase_price.toLocaleString()}` : '—'} />
        <Row label="Purchase date" value={formatDate(asset.purchase_date) || '—'} />

        <div className="mb-2.5 mt-5.5 text-[11px] font-semibold uppercase tracking-[.06em] text-ink-soft">
          Booking history
        </div>
        {loading ? (
          <div className="py-2 text-[12.5px] text-ink-soft">Loading…</div>
        ) : allocations.length ? (
          allocations.map((a) => (
            <div key={a.id} className="border-b border-border py-2.5 text-[12.5px]">
              <div className="flex justify-between">
                <span className="font-medium">{a.project_name}</span>
                <span className="text-ink-soft">
                  {formatDate(a.date_out)} – {formatDate(a.date_in)}
                </span>
              </div>
              <div className="mt-0.5 flex items-center justify-between text-ink-soft">
                <span>{allocationStatusLabel[a.status]}</span>
                {a.damage_flag && <span className="font-semibold text-red">Damage reported</span>}
              </div>
            </div>
          ))
        ) : (
          <div className="border-b border-border py-2.5 text-[12.5px] text-ink-soft">No bookings on record</div>
        )}

        <div className="mb-2.5 mt-5.5 text-[11px] font-semibold uppercase tracking-[.06em] text-ink-soft">
          Service records
        </div>
        {loading ? (
          <div className="py-2 text-[12.5px] text-ink-soft">Loading…</div>
        ) : serviceRecords.length ? (
          serviceRecords.map((s) => (
            <div key={s.id} className="flex justify-between border-b border-border py-2.5 text-[12.5px]">
              <span className="font-medium">{s.fault_description}</span>
              <span className="text-ink-soft">{formatDate(s.date_reported)}</span>
            </div>
          ))
        ) : (
          <div className="border-b border-border py-2.5 text-[12.5px] text-ink-soft">No faults logged</div>
        )}
      </div>
    </div>
  )
}

function Row({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex items-center justify-between border-b border-border py-2.25 text-[13px]">
      <span className="text-ink-soft">{label}</span>
      <span className="font-medium">{value}</span>
    </div>
  )
}
