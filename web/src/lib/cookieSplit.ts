// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id

// Cookie key names the backend executors look for (grokExtractCookieValue in
// internal/proxy/executor/grok_web.go and the gemini executor). Keeping the
// mapping here means the split logic and the submit concatenation agree on
// which cookie goes into which column.
export interface CookieSplitResult {
	primary: string
	secondary: string
}

const GEMINI_PRIMARY = '__Secure-1PSID'
const GEMINI_SECONDARY = '__Secure-1PSIDTS'
const GROK_PRIMARY = 'sso'
const GROK_SECONDARY = 'sso-rw'
const GROK_EXTRA = 'cf_clearance'

// parseCookieMap parses a `name=value; name=value` string (or a bare value)
// into a lowercase-keyed map. Values may contain `=` (base64 padding).
function parseCookieMap(raw: string): Record<string, string> {
	const out: Record<string, string> = {}
	for (const part of raw.split(';')) {
		const trimmed = part.trim()
		const eq = trimmed.indexOf('=')
		if (eq <= 0) continue
		out[trimmed.slice(0, eq).trim().toLowerCase()] = trimmed.slice(eq + 1).trim()
	}
	return out
}

// splitCookieHeader parses a pasted cookie header or a bare cookie value into
// the primary and secondary cookies the provider's executor expects. Returns
// null when the primary cookie is not found (the operator pasted the wrong
// string, or the provider is not one of the dual-column providers).
export function splitCookieHeader(
	raw: string,
	provider: 'gemini-web' | 'grok-web'
): CookieSplitResult | null {
	const trimmed = raw.trim()
	if (!trimmed) return null

	const map = parseCookieMap(trimmed)
	let primaryKey: string, secondaryKey: string
	if (provider === 'gemini-web') {
		primaryKey = GEMINI_PRIMARY.toLowerCase()
		secondaryKey = GEMINI_SECONDARY.toLowerCase()
	} else {
		primaryKey = GROK_PRIMARY.toLowerCase()
		secondaryKey = GROK_SECONDARY.toLowerCase()
	}

	// A bare value (no `key=`) is assumed to be the primary cookie.
	if (!trimmed.includes('=') || !map[primaryKey]) {
		// The operator may have pasted just the primary cookie value.
		if (trimmed.includes('=') && map[primaryKey] === undefined) return null
		// Bare value: treat the whole string as the primary cookie value.
		if (!trimmed.includes('=')) {
			return { primary: trimmed, secondary: map[secondaryKey] ?? '' }
		}
	}

	const primary = map[primaryKey] ?? ''
	if (!primary) return null

	let secondary = map[secondaryKey] ?? ''
	if (provider === 'grok-web') {
		const extra = map[GROK_EXTRA.toLowerCase()]
		if (extra) {
			secondary = secondary ? `${secondary}; ${GROK_EXTRA}=${extra}` : `${GROK_EXTRA}=${extra}`
		}
	}

	return { primary, secondary }
}

// combineCookieFields joins the two column values into the `primary=...;
// secondary=...` format the backend executor parses. Used on submit.
export function combineCookieFields(primary: string, secondary: string): string {
	const p = primary.trim()
	const s = secondary.trim()
	if (!s) return p
	return `${p}; ${secondaryKeyLabel(s)}=${s}`
}

// secondaryKeyLabel infers the secondary cookie's key name from the value or
// from whether it already carries one. The backend parser reads name=value
// pairs, so the combined string needs the key label.
function secondaryKeyLabel(value: string): string {
	if (value.includes('=')) return value.split('=')[0].trim()
	return 'secondary'
}
