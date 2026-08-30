import { ImageIcon } from './icons'

// Product photo (brief §7) with a plain fallback icon for the ~50 products
// that never had one in CurrentRMS — the category name is already shown as
// a text label right next to every thumbnail, so a generic placeholder
// (rather than 16 bespoke per-category icons) doesn't lose information.
export function ProductThumbnail({ url, size = 'card' }: { url?: string; size?: 'card' | 'panel' }) {
  const dims = size === 'card' ? 'h-14 w-14' : 'h-28 w-28'
  if (url) {
    return (
      <img
        src={url}
        alt=""
        className={`${dims} shrink-0 rounded-control border border-border bg-off-white object-cover`}
      />
    )
  }
  return (
    <div className={`flex ${dims} shrink-0 items-center justify-center rounded-control border border-border bg-off-white text-ink-soft`}>
      <ImageIcon className={size === 'card' ? 'h-5 w-5' : 'h-8 w-8'} />
    </div>
  )
}
