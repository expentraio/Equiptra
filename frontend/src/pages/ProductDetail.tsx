import { useEffect, useRef, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { api } from '../lib/api'
import type { Product, ProductAssetItem } from '../types'
import { AssetTag } from '../components/AssetTag'
import { AssetDetailPanel } from '../components/AssetDetailPanel'
import { ProductThumbnail } from '../components/ProductThumbnail'
import { ProductFormModal } from '../components/ProductFormModal'
import { AssetFormModal } from '../components/AssetFormModal'
import { ChevronLeftIcon, EditIcon, PlusIcon } from '../components/icons'

const statusLabel: Record<ProductAssetItem['status'], string> = {
  active: 'Active',
  written_off: 'Written off',
  sold: 'Sold',
  missing: 'Missing',
}

const statusBadgeClass: Record<ProductAssetItem['status'], string> = {
  active: 'bg-teal-fill text-teal',
  written_off: 'bg-[#EDEBE4] text-ink-soft',
  sold: 'bg-[#EDEBE4] text-ink-soft',
  missing: 'bg-red-fill text-red',
}

function formatDate(d: string) {
  return new Date(d).toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })
}

export function ProductDetail() {
  const { id } = useParams<{ id: string }>()
  const [searchParams] = useSearchParams()
  const highlightAssetId = searchParams.get('highlight')

  const [product, setProduct] = useState<Product | null>(null)
  const [assets, setAssets] = useState<ProductAssetItem[]>([])
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState<ProductAssetItem | null>(null)
  const [showEditProduct, setShowEditProduct] = useState(false)
  const [showNewAsset, setShowNewAsset] = useState(false)
  const [editingAsset, setEditingAsset] = useState<ProductAssetItem | null>(null)
  const highlightRef = useRef<HTMLTableRowElement>(null)

  function reload() {
    if (!id) return
    setLoading(true)
    Promise.all([api.get<Product>(`/products/${id}`), api.get<ProductAssetItem[]>(`/products/${id}/assets`)])
      .then(([p, a]) => {
        setProduct(p)
        setAssets(a)
      })
      .finally(() => setLoading(false))
  }

  useEffect(reload, [id])

  useEffect(() => {
    if (highlightAssetId && highlightRef.current) {
      highlightRef.current.scrollIntoView({ behavior: 'smooth', block: 'center' })
    }
  }, [highlightAssetId, assets])

  if (loading) return <div className="text-[13px] text-ink-soft">Loading…</div>
  if (!product) return <div className="text-[13px] text-ink-soft">Product not found.</div>

  return (
    <div>
      <Link
        to="/products"
        className="mb-4 inline-flex items-center gap-1.5 text-[12.5px] font-medium text-ink-soft hover:text-ink"
      >
        <ChevronLeftIcon className="h-3.5 w-3.5" />
        Back to products
      </Link>

      <div className="mb-5.5 flex flex-wrap items-start justify-between gap-4">
        <div className="flex items-start gap-4">
          <ProductThumbnail url={product.image_url} size="panel" />
          <div>
            <h1 className="text-[21px] font-bold">{product.name}</h1>
            <p className="mt-1 text-[13px] text-ink-soft">
              {product.category}
              {product.manufacturer ? ` · ${product.manufacturer}` : ''}
            </p>
            <div className="mt-2.5 flex flex-wrap gap-x-5 gap-y-1 text-[12.5px] text-ink-soft">
              {product.weight_kg != null && <span>Weight: {product.weight_kg} kg</span>}
              <span>Origin: {product.country_of_origin_code || 'Not set'}</span>
              {product.is_accessory && <span className="italic">Accessory</span>}
            </div>
          </div>
        </div>
        <button
          onClick={() => setShowEditProduct(true)}
          className="flex items-center gap-1.5 rounded-control border border-border bg-surface px-3.5 py-2 text-[12.5px] font-medium text-ink-soft hover:border-teal hover:text-teal"
        >
          <EditIcon className="h-3.5 w-3.5" />
          Edit product
        </button>
      </div>

      <div className="mb-3 flex items-center justify-between">
        <div className="text-[11px] font-semibold uppercase tracking-[.06em] text-ink-soft">Assets</div>
        <button
          onClick={() => setShowNewAsset(true)}
          className="flex items-center gap-1.5 rounded-control bg-teal px-3.5 py-2 text-[12.5px] font-medium text-white hover:opacity-90"
        >
          <PlusIcon className="h-3.5 w-3.5" />
          Add asset
        </button>
      </div>

      <div className="overflow-hidden rounded-card border border-border bg-surface">
        <table className="w-full border-collapse">
          <thead>
            <tr>
              {['Asset', 'Serial Number', 'Location', 'Status', 'Current Booking', ''].map((h) => (
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
            {assets.map((asset) => {
              const isHighlighted = highlightAssetId === String(asset.id)
              return (
                <tr
                  key={asset.id}
                  ref={isHighlighted ? highlightRef : undefined}
                  onClick={() => setSelected(asset)}
                  className={`cursor-pointer hover:bg-off-white ${isHighlighted ? 'bg-teal-fill' : ''}`}
                >
                  <td className="border-b border-border px-4 py-3.25 text-[13px]">
                    <AssetTag number={asset.asset_number} />
                  </td>
                  <td className="border-b border-border px-4 py-3.25 text-[13px]">{asset.serial_number || '—'}</td>
                  <td className="border-b border-border px-4 py-3.25 text-[13px]">{asset.location || '—'}</td>
                  <td className="border-b border-border px-4 py-3.25 text-[13px]">
                    <span className={`rounded-full px-2.5 py-1 text-[11px] font-semibold ${statusBadgeClass[asset.status]}`}>
                      {statusLabel[asset.status]}
                    </span>
                  </td>
                  <td className="border-b border-border px-4 py-3.25 text-[13px] text-ink-soft">
                    {asset.current_allocations.length === 0 && '—'}
                    {asset.current_allocations.length === 1 && (
                      <>
                        {asset.current_allocations[0].project_name} ({formatDate(asset.current_allocations[0].date_out)} –{' '}
                        {formatDate(asset.current_allocations[0].date_in)})
                      </>
                    )}
                    {asset.current_allocations.length > 1 && (
                      <>{asset.current_allocations.length} units currently out</>
                    )}
                  </td>
                  <td className="border-b border-border px-4 py-3.25 text-right text-[12px]">
                    <button
                      onClick={(e) => {
                        e.stopPropagation()
                        setEditingAsset(asset)
                      }}
                      className="font-medium text-ink-soft hover:text-teal"
                    >
                      Edit
                    </button>
                  </td>
                </tr>
              )
            })}
            {assets.length === 0 && (
              <tr>
                <td colSpan={6} className="px-4 py-8 text-center text-[13px] text-ink-soft">
                  No individual units on record for this product.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {selected && <AssetDetailPanel asset={selected} onClose={() => setSelected(null)} />}

      {showEditProduct && (
        <ProductFormModal
          product={product}
          onClose={() => setShowEditProduct(false)}
          onSaved={() => {
            setShowEditProduct(false)
            reload()
          }}
        />
      )}

      {showNewAsset && (
        <AssetFormModal
          productId={product.id}
          onClose={() => setShowNewAsset(false)}
          onSaved={() => {
            setShowNewAsset(false)
            reload()
          }}
        />
      )}

      {editingAsset && (
        <AssetFormModal
          productId={product.id}
          asset={editingAsset}
          onClose={() => setEditingAsset(null)}
          onSaved={() => {
            setEditingAsset(null)
            reload()
          }}
        />
      )}
    </div>
  )
}
