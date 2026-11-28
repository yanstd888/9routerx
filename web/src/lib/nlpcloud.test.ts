// 9RouterX — pinned NLP Cloud metadata, catalog and glyph.
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

// OmniRoute 632d84f5: config/providers/registry/nlpcloud/index.ts and
// src/shared/constants/providers/apikey/specialty-media.ts.
describe('NLP Cloud pinned metadata', () => {
  test('publishes the Token-auth chatbot entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'nlpcloud')
    expect(provider?.name).toBe('NLP Cloud')
    expect(provider?.alias).toBe('nlpc')
    expect(provider?.color).toBe('#2196F3')
    expect(provider?.icon).toBe('psychology')
    expect(provider?.website).toBe('https://docs.nlpcloud.com')
    expect(provider?.authHint).toContain('Token')
    expect(PROVIDER_ID_TO_ALIAS['nlpcloud']).toBe('nlpc')
  })

  test('carries the five pinned chatbot models for id and alias', () => {
    const expected = ['chatdolphin', 'dolphin', 'finetuned-llama-3-70b', 'llama-3-1-405b', 'llama-3-8b-instruct']
    expect(getModelsByProviderId('nlpcloud').map((model) => model.id)).toEqual(expected)
    expect(getModelsByProviderId('nlpc').map((model) => model.id)).toEqual(expected)
  })

  test('ships the pin-derived 128px NLPC glyph', () => {
    expect(getIconPath('nlpcloud')).toBe('/providers/nlpcloud.png')
    expect(getIconPath('nlpc')).toBe('/providers/nlpcloud.png')
    const png = readFileSync(resolve(root, 'web/public/providers/nlpcloud.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
