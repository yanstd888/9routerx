// 9RouterX — pinned Token Kiosk metadata and glyph.
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

// OmniRoute 632d84f5: config/providers/registry/token-kiosk/index.ts and
// docs/reference/PROVIDER_REFERENCE.md (Token Kiosk row).
describe('Token Kiosk pinned metadata', () => {
  test('publishes the gateway metadata', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'token-kiosk')
    expect(provider?.name).toBe('Token Kiosk')
    expect(provider?.alias).toBe('tk')
    expect(provider?.color).toBe('#6366F1')
    expect(provider?.icon).toBe('hub')
    expect(provider?.website).toBe('https://agent-router.gaib.ai')
    expect(provider?.authHint).toContain('Bearer')
    expect(PROVIDER_ID_TO_ALIAS['token-kiosk']).toBe('tk')
  })

  test('carries the five pinned snapshot models', () => {
    expect(getModelsByProviderId('token-kiosk').map((model) => model.id)).toEqual([
      'claude-3-5-sonnet',
      'deepseek-v3',
      'deepseek-r1',
      'kimi-k1.5',
      'minimax-m6',
    ])
    expect(getModelsByProviderId('tk').map((model) => model.id)).toEqual([
      'claude-3-5-sonnet',
      'deepseek-v3',
      'deepseek-r1',
      'kimi-k1.5',
      'minimax-m6',
    ])
  })

  test('ships the pinned TKI glyph at 128px', () => {
    expect(getIconPath('token-kiosk')).toBe('/providers/token-kiosk.png')
    const png = readFileSync(resolve(root, 'web/public/providers/token-kiosk.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
