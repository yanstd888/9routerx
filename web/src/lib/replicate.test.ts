// 9RouterX — pinned Replicate metadata and genuine artwork evidence.
import { describe, expect, test } from 'bun:test'
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { getModelsByProviderId, PROVIDER_ID_TO_ALIAS } from './models'
import { __setProviderCatalogForTest, PROVIDER_CATALOG } from './providers'
import { parseProviderRegistry } from './providers.registry-test'

const root = resolve(import.meta.dirname, '../../..')
__setProviderCatalogForTest(
  parseProviderRegistry(readFileSync(resolve(root, 'internal/providers/provider_registry.go'), 'utf8')),
)

describe('Replicate pinned metadata', () => {
  test('keeps the two curated models and isolated API-key brand', () => {
    expect(PROVIDER_ID_TO_ALIAS.replicate).toBe('replicate')
    expect(getModelsByProviderId('replicate').map((model) => model.id)).toEqual([
      'meta/meta-llama-3-70b-instruct',
      'mistralai/mistral-7b-instruct-v0.2',
    ])
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'replicate')
    expect(provider?.name).toBe('Replicate')
    expect(provider?.category).toBe('apikey')
    expect(provider?.alias).toBe('replicate')
    expect(provider?.color).toBe('#EA2805')
    expect(provider?.icon).toBe('content_copy')
    // Upstream display declares llm+video; its videoRegistry marks video
    // unsupported, so this lane exposes only llm and implements no video
    // transport at all.
    expect(provider?.serviceKinds).toEqual(['llm'])
    expect(provider?.modelsFetcher).toBeUndefined()
  })

  test('ships the pinned 128px PNG under the byte budget', () => {
    const png = readFileSync(resolve(root, 'web/public/providers/replicate.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
    expect(createHash('sha256').update(png).digest('hex')).toBe(
      'e4d632d92e7ca776e8cc6b895d25393a43bd28053913d62b00b3a3714e275d62',
    )
  })
})
