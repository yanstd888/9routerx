// 9RouterX — pinned Poe API metadata, models and artwork.
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

// OmniRoute 632d84f5: config/providers/registry/poe/index.ts. The API-key
// lane is `poe`; the web-cookie GraphQL lane stays on the separate `poe-web`.
describe('Poe API pinned metadata', () => {
  test('publishes the documented API-key entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'poe')
    expect(provider?.name).toBe('Poe')
    expect(provider?.alias).toBe('poe')
    expect(provider?.color).toBe('#F97316')
    expect(provider?.icon).toBe('hub')
    expect(provider?.website).toBe('https://creator.poe.com/api-reference')
    expect(provider?.authHint).toContain('Bearer API key')
    expect(PROVIDER_ID_TO_ALIAS['poe']).toBe('poe')
  })

  test('carries the three pinned models with Claude on Messages', () => {
    const models = getModelsByProviderId('poe')
    expect(models.map((model) => model.id)).toEqual(['gpt-5.2', 'claude-opus-4.8', 'gemini-3.0-pro'])
    expect(models[1]?.supportedFormats).toEqual(['claude'])
    expect(models[0]?.supportedFormats).toEqual(['openai'])
    expect(models[2]?.supportedFormats).toEqual(['openai'])
  })

  test('ships the genuine pinned artwork', () => {
    expect(getIconPath('poe')).toBe('/providers/poe.png')
    const png = readFileSync(resolve(root, 'web/public/providers/poe.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
