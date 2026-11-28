// 9RouterX — pinned Adapta One metadata and glyph.
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

// OmniRoute 632d84f5: config/providers/registry/adapta-web/index.ts and
// src/shared/constants/providers/web-cookie.ts.
describe('Adapta One pinned metadata', () => {
  test('publishes the Clerk-backed web entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'adapta-web')
    expect(provider?.name).toBe('Adapta.org (Adapta One Web)')
    expect(provider?.alias).toBe('adp-web')
    expect(provider?.color).toBe('#6E3AD3')
    expect(provider?.icon).toBe('auto_awesome')
    expect(provider?.website).toBe('https://agent.adapta.one')
    expect(provider?.authHint).toContain('__client')
    expect(PROVIDER_ID_TO_ALIAS['adapta-web']).toBe('adp-web')
  })

  test('carries the seven pinned models for id and alias', () => {
    expect(getModelsByProviderId('adapta-web').map((model) => model.id)).toEqual([
      'adapta-one', 'adapta-gpt', 'adapta-claude', 'adapta-gemini',
      'adapta-grok', 'adapta-deepseek', 'adapta-llama',
    ])
    expect(getModelsByProviderId('adp-web').map((model) => model.id)).toEqual(
      getModelsByProviderId('adapta-web').map((model) => model.id),
    )
  })

  test('ships the pin-derived 128px AW glyph', () => {
    expect(getIconPath('adapta-web')).toBe('/providers/adapta-web.png')
    expect(getIconPath('adp-web')).toBe('/providers/adapta-web.png')
    const png = readFileSync(resolve(root, 'web/public/providers/adapta-web.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
