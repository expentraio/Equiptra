import { useState, type FormEvent } from 'react'
import { api, ApiError } from '../lib/api'
import type { Product } from '../types'
import { CATEGORIES } from '../constants'
import { CloseIcon } from './icons'

export function ProductFormModal({
  product,
  onClose,
  onSaved,
}: {
  product?: Product
  onClose: () => void
  onSaved: () => void
}) {
  const isEdit = !!product
  const [name, setName] = useState(product?.name ?? '')
  const [category, setCategory] = useState(product?.category ?? '')
  const [manufacturer, setManufacturer] = useState(product?.manufacturer ?? '')
  const [weightKg, setWeightKg] = useState(product?.weight_kg != null ? String(product.weight_kg) : '')
  const [countryOfOrigin, setCountryOfOrigin] = useState(product?.country_of_origin_code ?? '')
  const [barcode, setBarcode] = useState(product?.barcode ?? '')
  const [description, setDescription] = useState(product?.description ?? '')
  const [isAccessory, setIsAccessory] = useState(product?.is_accessory ?? false)
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      const body = {
        name,
        category: category || null,
        manufacturer: manufacturer || null,
        weight_kg: weightKg.trim() === '' ? null : Number(weightKg),
        country_of_origin_code: countryOfOrigin.trim() === '' ? null : countryOfOrigin.trim().toUpperCase(),
        is_accessory: isAccessory,
        barcode: barcode || null,
        // Not exposed in this simple form — preserve whatever the product
        // already had (photo upload has its own flow; active defaults on).
        image_url: product?.image_url ?? null,
        description: description || null,
        active: product?.active ?? true,
      }
      if (isEdit) {
        await api.put(`/products/${product.id}`, body)
      } else {
        await api.post('/products', body)
      }
      onSaved()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not save product')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[rgba(15,23,42,.35)] p-4" onClick={onClose}>
      <form
        onSubmit={submit}
        onClick={(e) => e.stopPropagation()}
        className="max-h-[90vh] w-full max-w-[460px] overflow-y-auto rounded-card border border-border bg-surface p-6"
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-[16px] font-bold">{isEdit ? 'Edit product' : 'New product'}</h2>
          <button type="button" onClick={onClose} className="p-1 text-ink-soft hover:text-ink">
            <CloseIcon className="h-4.5 w-4.5" />
          </button>
        </div>

        <div className="flex flex-col gap-3.5">
          <label className="flex flex-col gap-1.5 text-[13px] font-medium">
            Name
            <input
              required
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
            />
          </label>

          <div className="flex gap-3">
            <label className="flex flex-1 flex-col gap-1.5 text-[13px] font-medium">
              Category
              <select
                value={category}
                onChange={(e) => setCategory(e.target.value)}
                className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
              >
                <option value="">—</option>
                {CATEGORIES.map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex flex-1 flex-col gap-1.5 text-[13px] font-medium">
              Manufacturer
              <input
                value={manufacturer}
                onChange={(e) => setManufacturer(e.target.value)}
                className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
              />
            </label>
          </div>

          <div className="flex gap-3">
            <label className="flex flex-1 flex-col gap-1.5 text-[13px] font-medium">
              Weight (kg)
              <input
                type="number"
                step="any"
                min="0"
                value={weightKg}
                onChange={(e) => setWeightKg(e.target.value)}
                className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
              />
            </label>
            <label className="flex flex-1 flex-col gap-1.5 text-[13px] font-medium">
              Country of origin
              <input
                value={countryOfOrigin}
                onChange={(e) => setCountryOfOrigin(e.target.value)}
                placeholder="ISO-2, e.g. GB"
                maxLength={2}
                className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] uppercase outline-none focus:border-teal"
              />
            </label>
          </div>

          <label className="flex flex-col gap-1.5 text-[13px] font-medium">
            Barcode
            <input
              value={barcode}
              onChange={(e) => setBarcode(e.target.value)}
              className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
            />
          </label>

          <label className="flex flex-col gap-1.5 text-[13px] font-medium">
            Description
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
              className="rounded-control border border-border px-3.5 py-2.25 text-[13.5px] outline-none focus:border-teal"
            />
          </label>

          <label className="flex items-center gap-2 text-[13px] font-medium">
            <input type="checkbox" checked={isAccessory} onChange={(e) => setIsAccessory(e.target.checked)} />
            Accessory (indented/italicized on delivery notes)
          </label>

          {error && (
            <div className="rounded-control border border-red-fill bg-red-fill px-3.5 py-2.5 text-[13px] font-medium text-red">
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={submitting}
            className="mt-1 rounded-control bg-teal px-4 py-2.5 text-[13px] font-medium text-white hover:opacity-90 disabled:opacity-60"
          >
            {submitting ? 'Saving…' : isEdit ? 'Save changes' : 'Create product'}
          </button>
        </div>
      </form>
    </div>
  )
}
