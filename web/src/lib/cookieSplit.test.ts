// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
import { describe, expect, test } from 'vitest'
import { combineCookieFields, splitCookieHeader } from './cookieSplit'

describe('splitCookieHeader', () => {
	test('gemini-web: full header with both cookies', () => {
		const raw = '__Secure-1PSID=abc123; __Secure-1PSIDTS=sid-ts-1'
		const r = splitCookieHeader(raw, 'gemini-web')
		expect(r).toEqual({ primary: 'abc123', secondary: 'sid-ts-1' })
	})

	test('gemini-web: primary only (no secondary)', () => {
		const r = splitCookieHeader('__Secure-1PSID=abc123', 'gemini-web')
		expect(r).toEqual({ primary: 'abc123', secondary: '' })
	})

	test('gemini-web: primary missing → null', () => {
		expect(splitCookieHeader('__Secure-1PSIDTS=sid-ts-1', 'gemini-web')).toBeNull()
	})

	test('grok-web: full header with sso and sso-rw', () => {
		const raw = 'sso=grok-sso-val; sso-rw=grok-rw-val'
		const r = splitCookieHeader(raw, 'grok-web')
		expect(r).toEqual({ primary: 'grok-sso-val', secondary: 'grok-rw-val' })
	})

	test('grok-web: sso + sso-rw + cf_clearance (extra appended to secondary)', () => {
		const raw = 'sso=grok-sso-val; sso-rw=grok-rw-val; cf_clearance=cf-tok'
		const r = splitCookieHeader(raw, 'grok-web')
		expect(r?.primary).toBe('grok-sso-val')
		expect(r?.secondary).toContain('grok-rw-val')
		expect(r?.secondary).toContain('cf_clearance=cf-tok')
	})

	test('grok-web: bare value (no key=value) treated as primary', () => {
		const r = splitCookieHeader('just-a-raw-cookie-value', 'grok-web')
		expect(r).toEqual({ primary: 'just-a-raw-cookie-value', secondary: '' })
	})

	test('empty input → null', () => {
		expect(splitCookieHeader('', 'gemini-web')).toBeNull()
		expect(splitCookieHeader('  ', 'grok-web')).toBeNull()
	})

	test('unrelated cookies → null (primary not found)', () => {
		expect(splitCookieHeader('other=value; more=stuff', 'gemini-web')).toBeNull()
	})
})

describe('combineCookieFields', () => {
	test('primary only (no secondary)', () => {
		expect(combineCookieFields('abc123', '')).toBe('abc123')
	})

	test('primary + secondary joined with key label', () => {
		expect(combineCookieFields('abc123', 'sid-ts-1')).toContain('abc123')
		expect(combineCookieFields('abc123', 'sid-ts-1')).toContain('sid-ts-1')
	})
})
