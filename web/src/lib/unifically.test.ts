// 9RouterX — pinned Unifically metadata and glyph.
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

// OmniRoute 632d84f5: config/providers/registry/unifically/index.ts and
// docs/reference/PROVIDER_REFERENCE.md (Unifically row).
describe('Unifically pinned metadata', () => {
  test('publishes the passthrough aggregator metadata', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'unifically')
    expect(provider?.name).toBe('Unifically')
    expect(provider?.alias).toBe('unifically')
    expect(provider?.color).toBe('#10B981')
    expect(provider?.icon).toBe('hub')
    expect(provider?.website).toBe('https://unifically.com')
    expect(provider?.notice?.text).toContain('category=llm')
    expect(PROVIDER_ID_TO_ALIAS['unifically']).toBe('unifically')
  })

  test('carries a passthrough-only catalog (no pinned static models)', () => {
    expect(getModelsByProviderId('unifically')).toEqual([])
  })

  test('ships the pinned UNI glyph at 128px', () => {
    expect(getIconPath('unifically')).toBe('/providers/unifically.png')
    const png = readFileSync(resolve(root, 'web/public/providers/unifically.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
