// 9RouterX — pinned Charm Hyper metadata and artwork.
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

// OmniRoute 632d84f5: config/providers/registry/charm-hyper/index.ts and
// public/providers/charm-hyper.svg.
describe('Charm Hyper pinned metadata', () => {
  test('publishes the gateway metadata', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'charm-hyper')
    expect(provider?.name).toBe('Charm Hyper')
    expect(provider?.alias).toBe('charm-hyper')
    expect(provider?.color).toBe('#7C3AED')
    expect(provider?.icon).toBe('router')
    expect(provider?.website).toBe('https://hyper.charm.land')
    expect(provider?.authHint).toContain('Hypercredits')
    expect(PROVIDER_ID_TO_ALIAS['charm-hyper']).toBe('charm-hyper')
  })

  test('carries the pinned hyper/auto default and passthrough catalog', () => {
    expect(getModelsByProviderId('charm-hyper').map((model) => model.id)).toEqual(['hyper/auto'])
  })

  test('ships the pinned Charm Hyper mark at 128px', () => {
    expect(getIconPath('charm-hyper')).toBe('/providers/charm-hyper.png')
    const png = readFileSync(resolve(root, 'web/public/providers/charm-hyper.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
