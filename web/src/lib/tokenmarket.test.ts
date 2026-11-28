// 9RouterX — pinned Token Market metadata and glyph.
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { getIconPath } from '../components/connections/types'
import { getModelsByProviderId, PROVIDER_ID_TO_ALIAS } from './models'
import { __setProviderCatalogForTest, PROVIDER_CATALOG } from './providers'
import { parseProviderRegistry } from './providers.registry-test'

const root = resolve(import.meta.dirname, '../../..')
__setProviderCatalogForTest(
  parseProviderRegistry(readFileSync(resolve(root, 'internal/providers/provider_registry.go'), 'utf8')),
)

// OmniRoute 632d84f5: config/providers/registry/tokenmarket/index.ts and
// docs/reference/PROVIDER_REFERENCE.md (Token Market row).
describe('Token Market pinned metadata', () => {
  test('publishes the passthrough aggregator metadata', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'tokenmarket')
    expect(provider?.name).toBe('Token Market')
    expect(provider?.alias).toBe('tokenmarket')
    expect(provider?.color).toBe('#2563EB')
    expect(provider?.icon).toBe('hub')
    expect(provider?.website).toBe('https://www.tokensmarket.ai')
    expect(provider?.authHint).toContain('Bearer token')
    expect(PROVIDER_ID_TO_ALIAS['tokenmarket']).toBe('tokenmarket')
  })

  test('carries a passthrough-only catalog (no pinned static models)', () => {
    expect(getModelsByProviderId('tokenmarket')).toEqual([])
  })

  test('ships the pinned TM glyph at 128px', () => {
    expect(getIconPath('tokenmarket')).toBe('/providers/tokenmarket.png')
    const png = readFileSync(resolve(root, 'web/public/providers/tokenmarket.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
