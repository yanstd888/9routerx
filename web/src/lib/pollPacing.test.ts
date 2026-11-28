// 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id
import { describe, expect, test } from 'bun:test'
import {
	BREAKDOWN_RENDER_CAP,
	isLongRange,
	nextVisibleCount,
	pollIntervals,
	SHORT_POLLS,
	LONG_POLLS,
	visibleRows,
} from './pollPacing'

describe('poll pacing (R5)', () => {
	test('short windows keep the fast tick', () => {
		for (const period of ['today', '24h', '6h', '5d', '1d', '', 'garbage']) {
			expect(pollIntervals(period)).toEqual({ totalsMs: 5000, breakdownMs: 30000 })
			expect(isLongRange(period)).toBe(false)
		}
	})

	test('a week or more slows both schedules', () => {
		for (const period of ['7d', '30d', '60d', '90d', '365d', 'all']) {
			expect(pollIntervals(period)).toEqual({ totalsMs: 15000, breakdownMs: 60000 })
			expect(isLongRange(period)).toBe(true)
		}
	})

	test('the day boundary sits at seven', () => {
		expect(isLongRange('6d')).toBe(false)
		expect(isLongRange('7d')).toBe(true)
	})

	test('the returned objects are copies, not shared constants', () => {
		const paced = pollIntervals('all')
		paced.totalsMs = 1
		expect(LONG_POLLS.totalsMs).toBe(15000)
		expect(SHORT_POLLS.totalsMs).toBe(5000)
	})
})

describe('breakdown render cap (R5)', () => {
	test('the rendered page is capped and grows on demand', () => {
		const rows = Array.from({ length: 450 }, (_, i) => ({ id: i }))
		expect(visibleRows(rows, BREAKDOWN_RENDER_CAP).length).toBe(200)
		expect(visibleRows(rows, 450).length).toBe(450)
		expect(visibleRows([], BREAKDOWN_RENDER_CAP).length).toBe(0)
	})

	test('show more advances by one cap and clamps to the row count', () => {
		expect(nextVisibleCount(200, 450)).toBe(400)
		expect(nextVisibleCount(400, 450)).toBe(450)
		expect(nextVisibleCount(450, 450)).toBe(450)
		expect(nextVisibleCount(0, 0)).toBe(0)
	})

	test('short tables render whole', () => {
		const rows = Array.from({ length: 30 }, (_, i) => ({ id: i }))
		expect(visibleRows(rows, BREAKDOWN_RENDER_CAP).length).toBe(30)
		expect(nextVisibleCount(BREAKDOWN_RENDER_CAP, 30)).toBe(30)
	})
})
