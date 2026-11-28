// 9RouterX — pinned Haiper metadata and glyph.
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

// OmniRoute 632d84f5: config/providers/registry/haiper/index.ts, config/imageRegistryData.ts,
// config/videoRegistry.ts and src/shared/constants/providers/apikey/specialty-media.ts.
describe('Haiper pinned metadata', () => {
  test('publishes the HAIPER_KEY media entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'haiper')
    expect(provider?.name).toBe('Haiper')
    expect(provider?.alias).toBe('hp')
    expect(provider?.color).toBe('#6366F1')
    expect(provider?.icon).toBe('videocam')
    expect(provider?.website).toBe('https://haiper.ai')
    expect(provider?.authHint).toContain('HAIPER_KEY')
    expect(PROVIDER_ID_TO_ALIAS['haiper']).toBe('hp')
  })

  test('carries the two pinned image and video models for id and alias', () => {
    expect(getModelsByProviderId('haiper').map((model) => model.id)).toEqual(['gen2', 'gen2-image'])
    expect(getModelsByProviderId('hp').map((model) => model.id)).toEqual(['gen2', 'gen2-image'])
  })

  test('ships the pin-derived 128px HP glyph', () => {
    expect(getIconPath('haiper')).toBe('/providers/haiper.png')
    expect(getIconPath('hp')).toBe('/providers/haiper.png')
    const png = readFileSync(resolve(root, 'web/public/providers/haiper.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
