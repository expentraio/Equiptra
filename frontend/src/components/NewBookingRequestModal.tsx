import { useEffect, useState } from 'react'
import { api, ApiError } from '../lib/api'
import type { BookingRequest, Product, Project } from '../types'
import { CloseIcon } from './icons'

function toDateInput(iso: string) {
  return iso.slice(0, 10)
}

export function NewBookingRequestModal({
  project,
  onClose,
  onCreated,
}: {
  project: Project
  onClose: () => void
  onCreated: () => void
}) {
  const [mode, setMode] = useState<'product' | 'placeholder'>('product')
  const [productQuery, setProductQuery] = useState('')
  const [productResults, setProductResults] = useState<Product[]>([])
  const [selectedProduct, setSelectedProduct] = useState<Product | null>(null)
  const [placeholderDescription, setPlaceholderDescription] = useState('')
  const [quantity, setQuantity] = useState(1)
  const [dateOut, setDateOut] = useState(toDateInput(project.start_date))
  const [dateIn, setDateIn] = useState(toDateInput(project.end_date))
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (!productQuery || selectedProduct) {
      setProductResults([])
      return
    }
    const timeout = setTimeout(() => {
      api
        .get<Product[]>(`/products?search=${encodeURIComponent(productQuery)}`)
        .then((results) => setProductResults(results.slice(0, 8)))
        .catch(() => {})
    }, 200)
    return () => clearTimeout(timeout)
  }, [productQuery, selectedProduct])

  async function submit() {
    if (mode === 'product' && !selectedProduct) {
      setError('Pick a product, or switch to "not yet decided"')
      return
    }
    if (mode === 'placeholder' && !placeholderDescription.trim()) {
      setError('Describe what you need')
      return
    }
    setError(null)
    setSubmitting(true)
    try {
      await api.post<BookingRequest>('/booking-requests', {
        project_id: project.id,
        product_id: mode === 'product' ? selectedProduct!.id : null,
        placeholder_description: mode === 'placeholder' ? placeholderDescription : null,
        quantity_requested: quantity,
        date_out: new Date(dateOut).toISOString(),
        date_in: new Date(dateIn).toISOString(),
      })
      onCreated()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not create booking request')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[rgba(15,23,42,.35)] p-4" onClick={onClose}>
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-[460px] rounded-card border border-border bg-surface p-6"
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-[16px] font-bold">Reserve for this project</h2>
          <button onClick={onClose} className="p-1 text-ink-soft hover:text-ink">
            <CloseIcon className="h-4.5 w-4.5" />
          </button>
        </div>

        <div className="mb-4 flex gap-2">
          <button
            onClick={() => setMode('product')}
            className={`flex-1 rounded-control border px-3 py-2 text-[12.5px] font-medium ${
              mode === 'product' ? 'border-ink bg-ink text-white' : 'border-border text-ink-soft'
            }`}
          >
            Pick a product
          </button>
          <button
            onClick={() => setMode('placeholder')}
            className={`flex-1 rounded-control border px-3 py-2 text-[12.5px] font-medium ${
              mode === 'placeholder' ? 'border-ink bg-ink text-white' : 'border-border text-ink-soft'
            }`}
          >
            Not decided yet
          </button>
        </div>

        <div className="flex flex-col gap-3.5">
          {mode === 'product' ? (
            <div className="flex flex-col gap-1.5 text-[13px] font-medium">
              Product
              {selectedProduct ? (
                <div className="flex items-center justify-between rounded-control border border-border px-3.5 py-2.25">
                  <span>
                    {selectedProduct.name}
                    <span className="ml-1.5 text-ink-soft">· {selectedProduct.category}</span>
                  </span>
                  <button
                    type="button"
                    onClick={() => {
                      setSelectedProduct(null)
                      setProductQuery('')
                    }}
                    className="text-[12px] font-medium text-teal"
                  >
                    Change
                  </button>
                </div>
              ) : (
                <div className="relative">
                  <input
                    autoFocus
                    value={productQuery}
                    onChange={(e) => setProductQuery(e.target.value)}
                    placeholder="Search products…"
                    className="w-full rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
                  />
                  {productResults.length > 0 && (
                    <div className="absolute z-10 mt-1 w-full rounded-control border border-border bg-surface shadow-sm">
                      {productResults.map((p) => (
                        <button
                          type="button"
                          key={p.id}
                          onClick={() => {
                            setSelectedProduct(p)
                            setProductResults([])
                          }}
                          className="flex w-full items-center justify-between px-3.5 py-2.25 text-left text-[13px] hover:bg-off-white"
                        >
                          <span>{p.name}</span>
                          <span className="text-ink-soft">{p.category}</span>
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </div>
          ) : (
            <label className="flex flex-col gap-1.5 text-[13px] font-medium">
              What do you need?
              <input
                autoFocus
                value={placeholderDescription}
                onChange={(e) => setPlaceholderDescription(e.target.value)}
                placeholder="e.g. some XLR cable, ~10x100m"
                className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
              />
            </label>
          )}

          <label className="flex flex-col gap-1.5 text-[13px] font-medium">
            Quantity
            <input
              type="number"
              min={1}
              value={quantity}
              onChange={(e) => setQuantity(Math.max(1, Number(e.target.value)))}
              className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
            />
          </label>

          <div className="flex gap-3">
            <label className="flex flex-1 flex-col gap-1.5 text-[13px] font-medium">
              Date out
              <input
                type="date"
                value={dateOut}
                onChange={(e) => setDateOut(e.target.value)}
                className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
              />
            </label>
            <label className="flex flex-1 flex-col gap-1.5 text-[13px] font-medium">
              Date in
              <input
                type="date"
                value={dateIn}
                onChange={(e) => setDateIn(e.target.value)}
                className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
              />
            </label>
          </div>

          {error && (
            <div className="rounded-control border border-red-fill bg-red-fill px-3.5 py-2.5 text-[13px] font-medium text-red">
              {error}
            </div>
          )}

          <button
            type="button"
            onClick={submit}
            disabled={submitting}
            className="mt-1 rounded-control bg-teal px-4 py-2.5 text-[13px] font-medium text-white hover:opacity-90 disabled:opacity-60"
          >
            {submitting ? 'Saving…' : 'Add to project'}
          </button>
        </div>
      </div>
    </div>
  )
}
