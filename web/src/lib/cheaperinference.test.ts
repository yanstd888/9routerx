// 9RouterX — pinned Cheaper Inference metadata, catalog and artwork.
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

// OmniRoute 632d84f5: config/providers/registry/cheaperinference/index.ts —
// catalog captured live upstream; the six GPT-5.x models ride the stateless
// /v1/responses surface via the per-model targetFormat tag.
const RESPONSES_MODELS = [
  'gpt-5.4',
  'gpt-5.4-mini',
  'gpt-5.5',
  'gpt-5.6-luna',
  'gpt-5.6-sol',
  'gpt-5.6-terra',
]

describe('Cheaper Inference pinned metadata', () => {
  test('publishes the dual-surface gateway metadata', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'cheaperinference')
    expect(provider?.name).toBe('Cheaper Inference')
    expect(provider?.alias).toBe('cinf')
    expect(provider?.color).toBe('#31F889')
    expect(provider?.icon).toBe('savings')
    expect(provider?.website).toBe('https://cheaperinference.com')
    expect(provider?.notice?.text).toContain('store:false')
    expect(PROVIDER_ID_TO_ALIAS['cheaperinference']).toBe('cinf')
  })

  test('carries the 39 pinned text models', () => {
    const models = getModelsByProviderId('cheaperinference')
    expect(models.length).toBe(39)
    expect(models[0].id).toBe('aion-labs.aion-2-0')
    expect(models[models.length - 1].id).toBe('minimax-m2.7')
    expect(getModelsByProviderId('cinf').length).toBe(39)
  })

  test('keeps the six responses-surface models in the catalog', () => {
    const ids = getModelsByProviderId('cinf').map((model) => model.id)
    for (const id of RESPONSES_MODELS) {
      expect(ids).toContain(id)
    }
  })

  test('ships the pinned CI glyph at 128px', () => {
    expect(getIconPath('cheaperinference')).toBe('/providers/cheaperinference.png')
    const png = readFileSync(resolve(root, 'web/public/providers/cheaperinference.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
