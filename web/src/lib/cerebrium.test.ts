// 9RouterX — pinned Cerebrium deployment metadata and glyph.
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

// OmniRoute 632d84f5: config/providers/registry/cerebrium/index.ts and
// src/shared/constants/providers/apikey/inference-hosts.ts.
describe('Cerebrium pinned metadata', () => {
  test('publishes the deployment-URL requirement', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'cerebrium')
    expect(provider?.name).toBe('Cerebrium')
    expect(provider?.alias).toBe('cerebrium')
    expect(provider?.color).toBe('#EB3A6F')
    expect(provider?.icon).toBe('memory')
    expect(provider?.website).toBe('https://www.cerebrium.ai')
    expect(provider?.authHint).toContain('/run endpoint')
    expect(PROVIDER_ID_TO_ALIAS['cerebrium']).toBe('cerebrium')
  })

  test('carries the one pinned chat model', () => {
    expect(getModelsByProviderId('cerebrium').map((model) => model.id)).toEqual([
      'meta-llama/Meta-Llama-3.1-8B-Instruct',
    ])
  })

  test('ships the pin-derived 128px CB glyph', () => {
    expect(getIconPath('cerebrium')).toBe('/providers/cerebrium.png')
    const png = readFileSync(resolve(root, 'web/public/providers/cerebrium.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
