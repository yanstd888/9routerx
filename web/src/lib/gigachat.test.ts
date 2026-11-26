// 9RouterX — pinned GigaChat metadata and glyph.
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

// OmniRoute 632d84f5: config/providers/registry/gigachat/index.ts and
// src/shared/constants/providers/apikey/regional.ts.
describe('GigaChat pinned metadata', () => {
  test('publishes the Basic-to-Bearer entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'gigachat')
    expect(provider?.name).toBe('GigaChat (Sber)')
    expect(provider?.alias).toBe('gigachat')
    expect(provider?.color).toBe('#10B981')
    expect(provider?.icon).toBe('lock_person')
    expect(provider?.website).toBe('https://developers.sber.ru')
    expect(provider?.authHint).toContain('Basic-encoded')
    expect(PROVIDER_ID_TO_ALIAS['gigachat']).toBe('gigachat')
  })

  test('carries the three pinned models', () => {
    expect(getModelsByProviderId('gigachat').map((model) => model.id)).toEqual([
      'GigaChat-2-Max',
      'GigaChat-2-Pro',
      'GigaChat-2-Lite',
    ])
  })

  test('ships the pin-derived 128px GC glyph', () => {
    expect(getIconPath('gigachat')).toBe('/providers/gigachat.png')
    const png = readFileSync(resolve(root, 'web/public/providers/gigachat.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
