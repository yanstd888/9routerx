// 9RouterX — pinned Conol metadata and glyph.
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

// OmniRoute 632d84f5: config/providers/registry/conol-web/index.ts and
// src/shared/constants/providers/web-cookie.ts.
describe('Conol pinned metadata', () => {
  test('publishes the session-cookie experimental entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'conol-web')
    expect(provider?.name).toBe('Conol (Unofficial/Experimental)')
    expect(provider?.alias).toBe('cnl')
    expect(provider?.color).toBe('#F6C945')
    expect(provider?.icon).toBe('auto_awesome')
    expect(provider?.website).toBe('https://conol.ai')
    expect(provider?.authHint).toContain('__Secure-better-auth.session_token')
    expect(PROVIDER_ID_TO_ALIAS['conol-web']).toBe('cnl')
  })

  test('carries the pinned fallback models for id and alias', () => {
    const ids = getModelsByProviderId('conol-web').map((model) => model.id)
    expect(ids.length).toBeGreaterThan(25)
    expect(ids).toContain('claude-sonnet-5')
    expect(ids).toContain('xiaomi/mimo-v2.5')
    expect(getModelsByProviderId('cnl').map((model) => model.id)).toEqual(ids)
  })

  test('ships the pin-derived 128px CO glyph', () => {
    expect(getIconPath('conol-web')).toBe('/providers/conol-web.png')
    expect(getIconPath('cnl')).toBe('/providers/conol-web.png')
    const png = readFileSync(resolve(root, 'web/public/providers/conol-web.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
