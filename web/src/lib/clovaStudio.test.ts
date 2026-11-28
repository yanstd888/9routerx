// 9RouterX — pinned Naver CLOVA Studio metadata and glyph.
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

// OmniRoute 632d84f5: config/providers/registry/clova-studio/index.ts and
// src/shared/constants/providers/apikey/regional.ts.
describe('Naver CLOVA Studio pinned metadata', () => {
  test('publishes the native v3 entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'clova-studio')
    expect(provider?.name).toBe('Naver CLOVA Studio')
    expect(provider?.alias).toBe('clova')
    expect(provider?.color).toBe('#03C75A')
    expect(provider?.icon).toBe('auto_awesome')
    expect(provider?.website).toContain('ncloud-docs.com')
    expect(provider?.authHint).toContain('Bearer token')
    expect(PROVIDER_ID_TO_ALIAS['clova-studio']).toBe('clova')
  })

  test('carries the three pinned v3 models for id and alias', () => {
    expect(getModelsByProviderId('clova-studio').map((model) => model.id)).toEqual([
      'HCX-007',
      'HCX-005',
      'HCX-DASH-002',
    ])
    expect(getModelsByProviderId('clova').map((model) => model.id)).toEqual([
      'HCX-007',
      'HCX-005',
      'HCX-DASH-002',
    ])
  })

  test('ships the pin-derived 128px CS glyph', () => {
    expect(getIconPath('clova-studio')).toBe('/providers/clova-studio.png')
    expect(getIconPath('clova')).toBe('/providers/clova-studio.png')
    const png = readFileSync(resolve(root, 'web/public/providers/clova-studio.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
