// 9RouterX — pinned FriendliAI metadata and artwork.
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

// OmniRoute 632d84f5: config/providers/registry/friendliai/index.ts and
// public/providers/friendli.svg.
describe('FriendliAI pinned metadata', () => {
  test('publishes the serverless lane metadata', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'friendliai')
    expect(provider?.name).toBe('FriendliAI')
    expect(provider?.alias).toBe('friendli')
    expect(provider?.color).toBe('#EC4899')
    expect(provider?.icon).toBe('handshake')
    expect(provider?.website).toBe('https://friendli.ai')
    expect(provider?.notice?.text).toContain('serverless')
    expect(PROVIDER_ID_TO_ALIAS['friendliai']).toBe('friendli')
  })

  test('carries the pinned llama serverless pair', () => {
    expect(getModelsByProviderId('friendliai').map((model) => model.id)).toEqual([
      'meta-llama-3.1-70b-instruct',
      'meta-llama-3.1-8b-instruct',
    ])
    expect(getModelsByProviderId('friendli').map((model) => model.id)).toEqual([
      'meta-llama-3.1-70b-instruct',
      'meta-llama-3.1-8b-instruct',
    ])
  })

  test('ships the pinned Friendli mark at 128px', () => {
    expect(getIconPath('friendliai')).toBe('/providers/friendliai.png')
    const png = readFileSync(resolve(root, 'web/public/providers/friendliai.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
