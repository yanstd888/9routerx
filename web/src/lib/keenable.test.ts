// 9RouterX — pinned Keenable web search/fetch metadata.
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { getIconPath } from '../components/connections/types'
import { __setProviderCatalogForTest, PROVIDER_CATALOG } from './providers'
import { parseProviderRegistry } from './providers.registry-test'

const root = resolve(import.meta.dirname, '../../..')
__setProviderCatalogForTest(
  parseProviderRegistry(readFileSync(resolve(root, 'internal/providers/provider_registry.go'), 'utf8')),
)

// OmniRoute 632d84f5: open-sse/config/keenable.ts and the Keenable
// search/fetch executors. No chat model catalog is published.
describe('Keenable pinned metadata', () => {
  test('publishes the optional-key web tool provider', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'keenable')
    expect(provider?.name).toBe('Keenable')
    expect(provider?.alias).toBe('keenable')
    expect(provider?.color).toBe('#06B6D4')
    expect(provider?.icon).toBe('search')
    expect(provider?.website).toBe('https://docs.keenable.ai/api-reference')
    expect(provider?.authHint).toContain('rate limits')
  })

  test('ships the pin-derived 128px KE glyph', () => {
    expect(getIconPath('keenable')).toBe('/providers/keenable.png')
    const png = readFileSync(resolve(root, 'web/public/providers/keenable.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
