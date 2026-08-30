// The signature "physical asset label" chip — reused anywhere an asset is
// referenced (tables, cards, detail panels), per the brief.
export function AssetTag({ number }: { number?: string | null }) {
  if (!number) {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-[5px] bg-ink-soft/40 px-2.5 py-[3px] pl-2 text-[11.5px] font-medium tracking-wide text-white">
        <span className="h-[5px] w-[5px] shrink-0 rounded-full bg-white/55" />
        bulk
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1.5 rounded-[5px] bg-navy px-2.5 py-[3px] pl-2 text-[11.5px] font-medium tracking-wide text-white">
      <span className="h-[5px] w-[5px] shrink-0 rounded-full bg-white/55" />
      {number}
    </span>
  )
}
