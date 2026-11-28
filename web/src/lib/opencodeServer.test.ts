// 9RouterX — pinned OpenCode CLI Server metadata, model catalog and glyph.
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { getIconPath, getProviderGlyph } from '../components/connections/types'
import { getModelsByProviderId, PROVIDER_ID_TO_ALIAS } from './models'
import { __setProviderCatalogForTest, PROVIDER_CATALOG } from './providers'
import { parseProviderRegistry } from './providers.registry-test'

const root = resolve(import.meta.dirname, '../../..')
__setProviderCatalogForTest(
  parseProviderRegistry(readFileSync(resolve(root, 'internal/providers/provider_registry.go'), 'utf8')),
)

// OmniRoute 632d84f5: config/providers/registry/opencode-server/index.ts and
// src/shared/constants/providers/local.ts. The registry's apikey/bearer hint
// conflicts with PROVIDER_REFERENCE.md and the executor, which send no key.
describe('OpenCode CLI Server pinned metadata', () => {
  test('publishes the keyless local-server registry entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'opencode-server')
    expect(provider?.name).toBe('OpenCode CLI Server')
    expect(provider?.alias).toBe('ocs')
    expect(provider?.category).toBe('free')
    expect(provider?.authType).toBe('none')
    expect(provider?.noAuth).toBe(true)
    expect(provider?.color).toBe('#F59E0B')
    expect(provider?.icon).toBe('terminal')
    expect(provider?.website).toBe('https://opencode.ai/docs/server/')
    expect(provider?.authHint).toContain('No API key is sent')
    expect(PROVIDER_ID_TO_ALIAS['opencode-server']).toBe('ocs')
  })

  test('carries only the six pinned bare model ids', () => {
    const models = getModelsByProviderId('opencode-server')
    expect(models.map((model) => model.id)).toEqual([
      'big-pickle',
      'space-bunny-free',
      'muse-spark-1.3-contributor-free',
      'nemotron-3-ultra-free',
      'mimo-v2.6-flash-free',
      'deepseek-v4.1-flash-free',
    ])
    expect(getModelsByProviderId('ocs').map((model) => model.id)).toEqual(models.map((model) => model.id))
  })

  test('ships the pin-derived 128px OC glyph', () => {
    // The pin ships no artwork: textIcon "OC" and color #F59E0B are the only
    // brand evidence, so the asset is a generated glyph, not an unverified
    // historical PNG or another provider's mark.
    expect(getIconPath('opencode-server')).toBe('/providers/opencode-server.png')
    expect(getIconPath('ocs')).toBe('/providers/opencode-server.png')
    expect(getProviderGlyph('ocs').name).toBe('OpenCode CLI Server')
    expect(getProviderGlyph('ocs').color).toBe('#F59E0B')
    const png = readFileSync(resolve(root, 'web/public/providers/opencode-server.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
