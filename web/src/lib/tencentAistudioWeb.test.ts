// 9RouterX — pinned Tencent AI Studio metadata and artwork.
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

// OmniRoute 632d84f5: config/providers/registry/tencent-aistudio-web/index.ts and
// src/shared/constants/providers/web-cookie.ts.
describe('Tencent AI Studio pinned metadata', () => {
  test('publishes the free cookie entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'tencent-aistudio-web')
    expect(provider?.name).toBe('Tencent AI Studio (Free)')
    expect(provider?.alias).toBe('tasw')
    expect(provider?.color).toBe('#0052D9')
    expect(provider?.icon).toBe('auto_awesome')
    expect(provider?.website).toBe('https://aistudio.tencent.ai')
    expect(provider?.authHint).toContain('Cookie header')
    expect(PROVIDER_ID_TO_ALIAS['tencent-aistudio-web']).toBe('tasw')
  })

  test('carries the three pinned Hunyuan model ids for id and alias', () => {
    expect(getModelsByProviderId('tencent-aistudio-web').map((model) => model.id)).toEqual([
      'hy3-g',
      'hunyuan-default',
      'hunyuan-3d',
    ])
    expect(getModelsByProviderId('tasw').map((model) => model.id)).toEqual([
      'hy3-g',
      'hunyuan-default',
      'hunyuan-3d',
    ])
  })

  test('ships the genuine pinned artwork', () => {
    expect(getIconPath('tencent-aistudio-web')).toBe('/providers/tencent-aistudio-web.png')
    const png = readFileSync(resolve(root, 'web/public/providers/tencent-aistudio-web.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBeLessThanOrEqual(512)
    expect(png.readUInt32BE(20)).toBeLessThanOrEqual(512)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
