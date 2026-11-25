// 9RouterX — pinned Ideogram metadata, model catalog and fallback artwork.
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

// OmniRoute 632d84f5: registry/ideogram/index.ts and
// src/shared/constants/providers/apikey/specialty-media.ts.
describe('Ideogram pinned metadata', () => {
  test('publishes the API-key image-only registry entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'ideogram')
    expect(provider?.name).toBe('Ideogram')
    expect(provider?.alias).toBe('ideo')
    expect(provider?.category).toBe('apikey')
    expect(provider?.authType).toBe('apikey')
    expect(provider?.color).toBe('#EC4899')
    expect(provider?.icon).toBe('image')
    expect(provider?.serviceKinds).toEqual(['image'])
    expect(provider?.website).toBe('https://ideogram.ai')
    expect(provider?.authHint).toBe('Get API key at ideogram.ai/docs/api')
    expect(PROVIDER_ID_TO_ALIAS.ideogram).toBe('ideo')
  })

  test('carries only the two pinned image models', () => {
    const models = getModelsByProviderId('ideogram')
    expect(models.map((model) => model.id)).toEqual(['V_3', 'V_2A'])
    expect(models.map((model) => model.name)).toEqual(['Ideogram V3', 'Ideogram V2A'])
    expect(models.map((model) => model.kind)).toEqual(['image', 'image'])
  })

  test('ships the pin-derived 128px glyph artwork', () => {
    // The pin ships no artwork: textIcon "ID" and color #EC4899 are the only
    // brand evidence, so the asset is a glyph generated from those two values
    // rather than a copy of an unverified historical PNG.
    expect(getIconPath('ideogram')).toBe('/providers/ideogram.png')
    expect(getIconPath('ideo')).toBe('/providers/ideogram.png')
    const glyph = getProviderGlyph('ideo')
    expect(glyph.initials).toBe('ID')
    expect(glyph.color).toBe('#EC4899')
    const png = readFileSync(resolve(root, 'web/public/providers/ideogram.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
