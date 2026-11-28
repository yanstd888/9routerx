// 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id

// Poll pacing for the Overview's two stat schedules. A short window's KPI row
// is worth a five-second tick; a long window's aggregate is expensive to fold
// server-side (the caches make it cheap, but the payload still moves) and no
// operator watches a 30d total change second by second — so long ranges slow
// both schedules down.
export const SHORT_POLLS = { totalsMs: 5000, breakdownMs: 30000 } as const
export const LONG_POLLS = { totalsMs: 15000, breakdownMs: 60000 } as const

// isLongRange reports whether a period spans a week or more: the `all`
// shortcut and any <n>d form with n >= 7. Sub-day windows and short day
// counts keep the fast tick. Unrecognized input is short — the server falls
// back to 24h (see resolveUsagePeriod), so the client should pace for what is
// actually served.
export function isLongRange(period: string): boolean {
	if (!period) return false
	if (period === 'all') return true
	const match = /^(\d+)d$/.exec(period)
	if (!match) return false
	return Number.parseInt(match[1], 10) >= 7
}

// pollIntervals returns the two schedules for a period.
export function pollIntervals(period: string): { totalsMs: number; breakdownMs: number } {
	return isLongRange(period) ? { ...LONG_POLLS } : { ...SHORT_POLLS }
}

// BREAKDOWN_RENDER_CAP bounds how many breakdown rows the table renders at
// once: an all-time ledger can carry thousands of buckets, and rendering them
// in one pass blocks the main thread. The table shows one page and grows on
// demand, so nothing is hidden — it is just not built until it is asked for.
export const BREAKDOWN_RENDER_CAP = 200

// visibleRows slices the rendered page off the full sorted row list.
export function visibleRows<T>(rows: T[], visibleCount: number): T[] {
	return rows.slice(0, visibleCount)
}

// nextVisibleCount grows the rendered page by one cap for the Show more
// control, clamped to the row count.
export function nextVisibleCount(current: number, total: number): number {
	return Math.min(current + BREAKDOWN_RENDER_CAP, total)
}
