// 9RouterX — pinned UncloseAI metadata and glyph.
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

// OmniRoute 632d84f5: config/providers/registry/uncloseai/index.ts and
// docs/reference/PROVIDER_REFERENCE.md (UncloseAI row).
describe('UncloseAI pinned metadata', () => {
  test('publishes the anonymous free-lane metadata', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'uncloseai')
    expect(provider?.name).toBe('UncloseAI')
    expect(provider?.alias).toBe('unc')
    expect(provider?.color).toBe('#43A047')
    expect(provider?.icon).toBe('smart_toy')
    expect(provider?.website).toBe('https://uncloseai.com')
    expect(provider?.authHint).toContain('non-empty string')
    expect(provider?.notice?.text).toContain('public beta is currently closed')
    expect(PROVIDER_ID_TO_ALIAS['uncloseai']).toBe('unc')
  })

  test('carries the one pinned free model', () => {
    expect(getModelsByProviderId('uncloseai').map((model) => model.id)).toEqual([
      'turboderp/Qwen3.8-27B-exl3',
    ])
    expect(getModelsByProviderId('unc').map((model) => model.id)).toEqual([
      'turboderp/Qwen3.8-27B-exl3',
    ])
  })

  test('ships the site-colored 128px U glyph', () => {
    expect(getIconPath('uncloseai')).toBe('/providers/uncloseai.png')
    const png = readFileSync(resolve(root, 'web/public/providers/uncloseai.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
