// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
import { describe, expect, test } from 'vitest'
import { seriesRows, topFamilies, type TrendBucket } from './trend'

// The owner's live 30d window that froze the page: a real "other" family
// ranked inside the top 6 while claude+threads fall below the cutoff. The
// tail fold used to push a SECOND "other" entry, handing the chart's keyed
// each duplicate keys and killing the page's reactivity.
const OWNER_SHAPE: TrendBucket[] = [
  { label: 'Oct 16', tokens: 18_863_484, cost: 2.77, byModel: { deepseek: 808_374 } },
  { label: 'Oct 21', tokens: 69_773_298, cost: 37.02, byModel: { gemini: 69_000_000, other: 7_000, claude: 700, threads: 598 } },
  { label: 'Nov 5', tokens: 730_411_805, cost: 457.23, byModel: { gemini: 700_000_000, glm: 112_000_000, code: 3_000, gpt: 24_667 } },
  { label: 'Nov 12', tokens: 0, cost: 0, byModel: {} },
]

describe('topFamilies', () => {
  test('an existing "other" in the top families merges the tail instead of duplicating', () => {
    const families = topFamilies(OWNER_SHAPE)
    const others = families.filter((f) => f.family === 'other')
    expect(others).toHaveLength(1)
    // gemini + glm + deepseek + gpt + other + code kept; claude+threads folded in
    expect(families).toHaveLength(6)
    const names = families.map((f) => f.family)
    expect(new Set(names).size).toBe(names.length)
  })

  test('without a real "other" the tail still folds into one', () => {
    // MAX_FAMILIES is 6; eight single-family buckets force a fold.
    const many: TrendBucket[] = Array.from({ length: 8 }, (_, i) => ({
      label: `b${i}`,
      tokens: 10,
      cost: 0,
      byModel: { [`fam${i}`]: 10 - i },
    }))
    const folded = topFamilies(many)
    expect(folded.filter((f) => f.family === 'other')).toHaveLength(1)
    expect(folded).toHaveLength(7) // six kept families + the folded "other"
    const names = folded.map((f) => f.family)
    expect(new Set(names).size).toBe(names.length)
  })

  test('totals-only buckets plot as one synthetic "total" series', () => {
    const buckets: TrendBucket[] = [
      { label: 'a', tokens: 100, cost: 0, byModel: {} },
      { label: 'b', tokens: 200, cost: 0, byModel: {} },
    ]
    const families = topFamilies(buckets)
    expect(families).toHaveLength(1)
    expect(families[0].family).toBe('total')
    expect(families[0].total).toBe(300)
    const rows = seriesRows(buckets, families)
    expect(rows[0]['total']).toBe(100)
    expect(rows[1]['total']).toBe(200)
  })

  test('seriesRows aggregates real "other" together with the folded tail', () => {
    const rows = seriesRows(OWNER_SHAPE, topFamilies(OWNER_SHAPE))
    // Oct 21 row: kept families carry their own values; "other" carries the
    // real other bucket plus every non-kept family (claude, threads).
    const oct21 = rows[1] as Record<string, number>
    expect(oct21['other']).toBe(7_000 + 700 + 598)
    // gemini stays its own series
    expect(oct21['gemini']).toBe(69_000_000)
  })
})
