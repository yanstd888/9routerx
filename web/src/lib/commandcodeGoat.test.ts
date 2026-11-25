// 9RouterX — pinned Command Code Provider API registry, catalog and artwork.
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

// go9r 0cce0710: internal/providers/commandcode_goat_catalog.go snapshot
// 2026-11-11 and the registry/provider config entries beside it.
describe('Command Code Provider API pinned metadata', () => {
  test('publishes an isolated documented-API registry entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'commandcode-goat')
    expect(provider?.name).toBe('Command Code (Provider API)')
    expect(provider?.alias).toBe('cmc-goat')
    expect(provider?.color).toBe('#000000')
    expect(provider?.icon).toBe('smart_toy')
    expect(provider?.website).toBe('https://commandcode.ai/docs/provider')
    expect(provider?.serviceKinds).toContain('systemone')
    expect(provider?.authHint).toContain('/messages only')
    expect(PROVIDER_ID_TO_ALIAS['commandcode-goat']).toBe('cmc-goat')
  })

  test('carries the 97-model feed snapshot for id and alias', () => {
    const models = getModelsByProviderId('commandcode-goat')
    expect(models).toHaveLength(97)
    expect(models.slice(0, 3).map((model) => model.id)).toEqual([
      'MiniMaxAI/MiniMax-M2.5',
      'MiniMaxAI/MiniMax-M2.7',
      'MiniMaxAI/MiniMax-M3',
    ])
    expect(models.slice(-3).map((model) => model.id)).toEqual([
      'zai-org/GLM-5.2',
      'zai-org/GLM-5.2-Fast',
      'zai-org/GLM-5.3',
    ])
    expect(getModelsByProviderId('cmc-goat')).toHaveLength(97)
  })

  test('marks the twelve Claude-target models', () => {
    const models = getModelsByProviderId('commandcode-goat')
    expect(models.filter((model) => model.supportedFormats.includes('claude'))).toHaveLength(12)
    expect(models.filter((model) => model.supportedFormats.includes('openai'))).toHaveLength(85)
  })

  test('ships the pinned 128px artwork', () => {
    expect(getIconPath('commandcode-goat')).toBe('/providers/commandcode-goat.png')
    expect(getIconPath('cmc-goat')).toBe('/providers/commandcode-goat.png')
    const png = readFileSync(resolve(root, 'web/public/providers/commandcode-goat.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
