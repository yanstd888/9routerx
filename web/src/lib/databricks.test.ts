// 9RouterX — pinned Databricks Model Serving metadata and artwork.
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

// OmniRoute 632d84f5: config/providers/registry/databricks/index.ts; the
// workspace Base URL contract comes from the upstream dashboard example host
// (providerPageHelpers.ts) and the executor-default URL unit test.
describe('Databricks pinned metadata', () => {
  test('publishes the per-workspace serving contract', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'databricks')
    expect(provider?.name).toBe('Databricks')
    expect(provider?.alias).toBe('databricks')
    expect(provider?.color).toBe('#F97316')
    expect(provider?.icon).toBe('table_chart')
    expect(provider?.website).toBe('https://www.databricks.com')
    expect(provider?.authHint).toContain('serving-endpoints')
    expect(provider?.notice?.text).toContain('placeholder')
    expect(PROVIDER_ID_TO_ALIAS['databricks']).toBe('databricks')
  })

  test('carries the four pinned serving models', () => {
    expect(getModelsByProviderId('databricks').map((model) => model.id)).toEqual([
      'databricks-gpt-5',
      'databricks-meta-llama-3-3-70b-instruct',
      'databricks-claude-sonnet-4',
      'databricks-gemini-2-5-pro',
    ])
  })

  test('ships a 128px brand mark', () => {
    expect(getIconPath('databricks')).toBe('/providers/databricks.png')
    const png = readFileSync(resolve(root, 'web/public/providers/databricks.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
