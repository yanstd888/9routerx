// 9RouterX — all source code port and modification by https://github.com/yanstd888/ — https://t.me/aiclub_id
// Frontend aggregation for the shadcn dashboard charts (TASK-9routerX-shadcn-dashboard §2):
// pure functions over the EXISTING /api/usage/chart response — no new endpoints.

export interface TrendBucket {
  label: string
  tokens: number
  cost: number
  byModel: Record<string, number>
}

export interface TrendFamily {
  family: string
  total: number
  /** A CSS color: the shadcn chart slots, which the theme maps to askglyph hues. */
  color: string
}

// At most six plotted families, mirroring UsageChart's legend cap so every
// plotted color has a named entry and nothing is left unexplained.
export const MAX_FAMILIES = 6

// The shadcn chart slots resolve through --color-{key} (Chart.Style) to the
// per-theme --chart-N tokens, which index.css points at the askglyph palette.
const FAMILY_VARS = [
  'var(--chart-1)',
  'var(--chart-2)',
  'var(--chart-3)',
  'var(--chart-4)',
  'var(--chart-5)',
]

export const OTHER_COLOR = 'var(--muted-foreground)'

// topFamilies ranks model families by period total, keeps the top six and
// folds the tail into "other" so the legend stays bounded — the same shape
// UsageChart draws, kept identical so both charts tell one story.
export function topFamilies(buckets: TrendBucket[]): TrendFamily[] {
  const totals = new Map<string, number>()
  for (const b of buckets) {
    for (const [family, tokens] of Object.entries(b.byModel || {})) {
      totals.set(family, (totals.get(family) || 0) + (tokens || 0))
    }
  }
  const ranked = [...totals.entries()].sort((a, b) => b[1] - a[1])
  const kept = ranked.slice(0, MAX_FAMILIES)
  const tail = ranked.slice(MAX_FAMILIES)
  const series: TrendFamily[] = kept.map(([family, total], i) => ({
    family,
    total,
    color: FAMILY_VARS[i % FAMILY_VARS.length],
  }))
  if (tail.length > 0) {
    // The backend can fold tail families into an "other" bucket of its own,
    // so "other" may already sit in the kept series. Merging into it keeps
    // the family list unique: a duplicate would hand the chart's keyed each
    // two entries with the same key and kill the page's reactivity.
    const tailTotal = tail.reduce((sum, [, t]) => sum + t, 0)
    const existing = series.find((s) => s.family === 'other')
    if (existing) {
      existing.total += tailTotal
    } else {
      series.push({ family: 'other', total: tailTotal, color: OTHER_COLOR })
    }
  }
  // The bucket totals can exist without any family split at all (the chart
  // API falls back to raw history when the daily rollup is empty). Plot a
  // single synthetic "total" series instead of rendering an empty chart.
  if (series.length === 0 && buckets.some((b) => (b.tokens || 0) > 0)) {
    series.push({
      family: 'total',
      total: buckets.reduce((sum, b) => sum + (b.tokens || 0), 0),
      color: FAMILY_VARS[0],
    })
  }
  return series
}

// seriesRows pivots the buckets into one row per label with a column per kept
// family ("other" = the folded tail), the shape LayerChart's stacked area reads.
// When no bucket carries a family split, the row carries the bucket total as
// the synthetic "total" series instead.
export function seriesRows(buckets: TrendBucket[], families: TrendFamily[]): Record<string, string | number>[] {
  const kept = families.filter((f) => f.family !== 'other').map((f) => f.family)
  const totalsOnly = !buckets.some((b) => Object.values(b.byModel || {}).some((t) => (t || 0) > 0))
  return buckets.map((b) => {
    const row: Record<string, string | number> = { label: b.label }
    if (totalsOnly) {
      row['total'] = b.tokens || 0
      return row
    }
    for (const family of kept) {
      row[family] = b.byModel?.[family] || 0
    }
    row['other'] = Object.entries(b.byModel || {})
      .filter(([family]) => !kept.includes(family))
      .reduce((sum, [, t]) => sum + (t || 0), 0)
    return row
  })
}

// bucketDelta compares the second half of the window against the first half —
// the only honest "direction" a trailing window can state without a
// date-bounded query (the usage API speaks trailing windows only). Returns a
// percentage, or null when the first half is empty (a percentage of nothing).
export function bucketDelta(values: number[]): number | null {
  if (values.length < 2) return null
  const half = Math.floor(values.length / 2)
  const first = values.slice(0, half).reduce((s, v) => s + (v || 0), 0)
  const second = values.slice(half).reduce((s, v) => s + (v || 0), 0)
  if (first <= 0) return null
  return ((second - first) / first) * 100
}
