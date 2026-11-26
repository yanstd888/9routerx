// 9RouterX — pinned 1min.ai metadata and glyph.
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

// OmniRoute 632d84f5: config/providers/registry/oneminai/index.ts and
// src/shared/constants/providers/apikey/gateways.ts.
describe('1min.ai pinned metadata', () => {
  test('publishes the API-KEY prompt gateway', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'oneminai')
    expect(provider?.name).toBe('1min.AI')
    expect(provider?.alias).toBe('1min')
    expect(provider?.color).toBe('#6366F1')
    expect(provider?.icon).toBe('hub')
    expect(provider?.website).toBe('https://1min.ai')
    expect(provider?.authHint).toContain('API key')
    expect(PROVIDER_ID_TO_ALIAS['oneminai']).toBe('1min')
  })

  test('carries the pinned model for id and alias', () => {
    expect(getModelsByProviderId('oneminai').map((model) => model.id)).toEqual(['gpt-4o-mini'])
    expect(getModelsByProviderId('1min').map((model) => model.id)).toEqual(['gpt-4o-mini'])
  })

  test('ships the pin-derived 128px 1M glyph', () => {
    expect(getIconPath('oneminai')).toBe('/providers/oneminai.png')
    expect(getIconPath('1min')).toBe('/providers/oneminai.png')
    const png = readFileSync(resolve(root, 'web/public/providers/oneminai.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
