// 9RouterX — pinned Leonardo metadata, model catalog and generated glyph.
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { getIconPath, getProviderGlyph } from '../components/connections/types'
import { getModelsByProviderId, PROVIDER_ID_TO_ALIAS } from './models'
import { __setProviderCatalogForTest, PROVIDER_CATALOG } from './providers'
import { parseProviderRegistry } from './providers.registry-test'

const root = resolve(import.meta.dirname, '../../..')
__setProviderCatalogForTest(
  parseProviderRegistry(readFileSync(resolve(root, 'internal/providers/provider_registry.go'), 'utf8')),
)

// OmniRoute 632d84f5: config/providers/registry/leonardo/index.ts and
// src/shared/constants/providers/apikey/specialty-media.ts.
describe('Leonardo pinned metadata', () => {
  test('publishes the API-key image-only registry entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'leonardo')
    expect(provider?.name).toBe('Leonardo AI')
    expect(provider?.alias).toBe('leo')
    expect(provider?.category).toBe('apikey')
    expect(provider?.authType).toBe('apikey')
    expect(provider?.color).toBe('#8B5CF6')
    expect(provider?.icon).toBe('palette')
    expect(provider?.serviceKinds).toEqual(['image'])
    expect(provider?.website).toBe('https://leonardo.ai')
    expect(provider?.authHint).toBe('Get API key at leonardo.ai/developer')
    expect(PROVIDER_ID_TO_ALIAS.leonardo).toBe('leo')
  })

  test('carries only the two pinned image models', () => {
    const models = getModelsByProviderId('leonardo')
    expect(models.map((model) => model.id)).toEqual(['phoenix', 'sdxl'])
    expect(models.map((model) => model.name)).toEqual(['Phoenix', 'SDXL'])
    expect(models.map((model) => model.kind)).toEqual(['image', 'image'])
  })

  test('ships the pin-derived 128px glyph artwork', () => {
    // The pin ships no artwork: textIcon "LE" and color #8B5CF6 are the only
    // brand evidence, so the asset is a generated glyph, not an unverified
    // historical PNG or another provider's mark.
    expect(getIconPath('leonardo')).toBe('/providers/leonardo.png')
    expect(getIconPath('leo')).toBe('/providers/leonardo.png')
    expect(getProviderGlyph('leo').name).toBe('Leonardo AI')
    expect(getProviderGlyph('leo').color).toBe('#8B5CF6')
    const png = readFileSync(resolve(root, 'web/public/providers/leonardo.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
