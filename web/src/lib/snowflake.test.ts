// 9RouterX — pinned Snowflake Cortex metadata and artwork.
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

// OmniRoute 632d84f5: config/providers/registry/snowflake/index.ts and
// public/providers/snowflake.svg; the Cortex route and PAT/KEYPAIR_JWT
// classification come from the upstream executor-default unit tests.
describe('Snowflake Cortex pinned metadata', () => {
  test('publishes the Cortex inference contract', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'snowflake')
    expect(provider?.name).toBe('Snowflake Cortex')
    expect(provider?.alias).toBe('snowflake')
    expect(provider?.color).toBe('#29B5E8')
    expect(provider?.icon).toBe('ac_unit')
    expect(provider?.website).toBe('https://www.snowflake.com')
    expect(provider?.authHint).toContain('pat/')
    expect(provider?.notice?.text).toContain('inference:complete')
    expect(PROVIDER_ID_TO_ALIAS['snowflake']).toBe('snowflake')
  })

  test('carries the four pinned Cortex models', () => {
    expect(getModelsByProviderId('snowflake').map((model) => model.id)).toEqual([
      'llama3.1-70b',
      'llama3.3-70b',
      'deepseek-r1',
      'claude-3-5-sonnet',
    ])
  })

  test('ships the pinned Snowflake mark at 128px', () => {
    expect(getIconPath('snowflake')).toBe('/providers/snowflake.png')
    const png = readFileSync(resolve(root, 'web/public/providers/snowflake.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
