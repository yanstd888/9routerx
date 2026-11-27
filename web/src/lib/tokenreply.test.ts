// 9RouterX — pinned TokenReply metadata and glyph.
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

// OmniRoute 632d84f5: config/providers/registry/tokenreply/index.ts and
// docs/reference/PROVIDER_REFERENCE.md (TokenReply row).
describe('TokenReply pinned metadata', () => {
  test('publishes the passthrough aggregator metadata', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'tokenreply')
    expect(provider?.name).toBe('TokenReply')
    expect(provider?.alias).toBe('tokenreply')
    expect(provider?.color).toBe('#3B82F6')
    expect(provider?.icon).toBe('router')
    expect(provider?.website).toBe('https://www.tokenreply.com')
    expect(provider?.notice?.text).toContain('daily limits')
    expect(PROVIDER_ID_TO_ALIAS['tokenreply']).toBe('tokenreply')
  })

  test('carries a passthrough-only catalog (no pinned static models)', () => {
    expect(getModelsByProviderId('tokenreply')).toEqual([])
  })

  test('ships the pinned TR glyph at 128px', () => {
    expect(getIconPath('tokenreply')).toBe('/providers/tokenreply.png')
    const png = readFileSync(resolve(root, 'web/public/providers/tokenreply.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
