// 9RouterX — pinned NoTrack Web metadata and artwork.
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

// OmniRoute 632d84f5: config/providers/registry/notrack-web/index.ts and
// src/shared/constants/providers/web-cookie-notrack.ts.
describe('NoTrack Web pinned metadata', () => {
  test('publishes the free session-cookie entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'notrack-web')
    expect(provider?.name).toBe('NoTrack Web (Free)')
    expect(provider?.alias).toBe('ntw')
    expect(provider?.color).toBe('#0EA5E9')
    expect(provider?.icon).toBe('auto_awesome')
    expect(provider?.website).toBe('https://notrack.ai')
    expect(provider?.authHint).toContain('si_usr_id')
    expect(PROVIDER_ID_TO_ALIAS['notrack-web']).toBe('ntw')
  })

  test('carries the one pinned model for id and alias', () => {
    expect(getModelsByProviderId('notrack-web').map((model) => model.id)).toEqual(['notrack-c'])
    expect(getModelsByProviderId('ntw').map((model) => model.id)).toEqual(['notrack-c'])
  })

  test('ships the genuine pinned artwork', () => {
    expect(getIconPath('notrack-web')).toBe('/providers/notrack-web.png')
    expect(getIconPath('ntw')).toBe('/providers/notrack-web.png')
    const png = readFileSync(resolve(root, 'web/public/providers/notrack-web.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBeLessThanOrEqual(512)
    expect(png.readUInt32BE(20)).toBeLessThanOrEqual(512)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
