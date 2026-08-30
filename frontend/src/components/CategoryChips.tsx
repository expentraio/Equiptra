import { CATEGORIES } from '../constants'

// Shared category filter chips — used on the Products browse page and
// anywhere else a product needs to be found by category (e.g. picking a
// product while reserving for a project).
export function CategoryChips({
  active,
  onChange,
}: {
  active: string | null
  onChange: (category: string | null) => void
}) {
  return (
    <div className="flex flex-wrap gap-2">
      <Chip active={active === null} onClick={() => onChange(null)}>
        All
      </Chip>
      {CATEGORIES.map((c) => (
        <Chip key={c} active={active === c} onClick={() => onChange(c)}>
          {c}
        </Chip>
      ))}
    </div>
  )
}

function Chip({ active, onClick, children }: { active: boolean; onClick: () => void; children: string }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`rounded-full border px-3.25 py-1.5 text-[12.5px] font-medium ${
        active ? 'border-ink bg-ink text-white' : 'border-border bg-surface text-ink-soft'
      }`}
    >
      {children}
    </button>
  )
}
