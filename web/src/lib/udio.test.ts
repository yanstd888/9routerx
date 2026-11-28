// 9RouterX — pinned Udio music metadata and artwork.
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

// OmniRoute 632d84f5: config/providers/registry/udio/index.ts, config/musicRegistry.ts
// and src/shared/constants/providers/apikey/specialty-media.ts.
describe('Udio pinned metadata', () => {
  test('publishes the session-cookie music entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'udio')
    expect(provider?.name).toBe('Udio')
    expect(provider?.alias).toBe('udio')
    expect(provider?.color).toBe('#10B981')
    expect(provider?.icon).toBe('music_note')
    expect(provider?.website).toBe('https://udio.com')
    expect(provider?.authHint).toContain('session cookie')
    expect(PROVIDER_ID_TO_ALIAS['udio']).toBe('udio')
  })

  test('carries the one pinned music model', () => {
    expect(getModelsByProviderId('udio').map((model) => model.id)).toEqual(['udio-default'])
  })

  test('ships the genuine pinned artwork', () => {
    expect(getIconPath('udio')).toBe('/providers/udio.png')
    const png = readFileSync(resolve(root, 'web/public/providers/udio.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBeLessThanOrEqual(512)
    expect(png.readUInt32BE(20)).toBeLessThanOrEqual(512)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
