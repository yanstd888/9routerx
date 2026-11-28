// 9RouterX — pinned SYNTX.ai metadata and glyph.
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

// OmniRoute 632d84f5: config/providers/registry/syntx/index.ts and
// src/shared/constants/providers/web-cookie.ts.
describe('SYNTX.ai pinned metadata', () => {
  test('publishes the Bearer JWT experimental entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'syntx')
    expect(provider?.name).toBe('SYNTX.ai (Unofficial/Experimental)')
    expect(provider?.alias).toBe('stx')
    expect(provider?.color).toBe('#FF4D00')
    expect(provider?.icon).toBe('auto_awesome')
    expect(provider?.website).toBe('https://syntx.ai')
    expect(provider?.authHint).toContain('Bearer JWT')
    expect(PROVIDER_ID_TO_ALIAS['syntx']).toBe('stx')
  })

  test('carries the pinned fallback models for id and alias', () => {
    const ids = getModelsByProviderId('syntx').map((model) => model.id)
    expect(ids.length).toBeGreaterThan(30)
    expect(ids).toContain('claude-fable-5-1')
    expect(ids).toContain('glm-5.2')
    expect(getModelsByProviderId('stx').map((model) => model.id)).toEqual(ids)
  })

  test('ships the pin-derived 128px SX glyph', () => {
    expect(getIconPath('syntx')).toBe('/providers/syntx.png')
    expect(getIconPath('stx')).toBe('/providers/syntx.png')
    const png = readFileSync(resolve(root, 'web/public/providers/syntx.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
