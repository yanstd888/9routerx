// 9RouterX — pinned HuggingChat metadata and artwork.
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

// OmniRoute 632d84f5: config/providers/registry/huggingchat/index.ts and
// src/shared/constants/providers/web-cookie.ts.
describe('HuggingChat pinned metadata', () => {
  test('publishes the free session-cookie entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'huggingchat')
    expect(provider?.name).toBe('HuggingChat (Free)')
    expect(provider?.alias).toBe('huggingchat')
    expect(provider?.color).toBe('#FFD21E')
    expect(provider?.icon).toBe('auto_awesome')
    expect(provider?.website).toBe('https://huggingface.co/chat')
    expect(provider?.authHint).toContain('hf-chat')
    expect(PROVIDER_ID_TO_ALIAS['huggingchat']).toBe('huggingchat')
  })

  test('carries the 24 pinned free chat models', () => {
    const models = getModelsByProviderId('huggingchat')
    expect(models).toHaveLength(24)
    expect(models[0]?.id).toBe('baidu/ERNIE-4.5-VL-424B-A47B-Base-PT')
    expect(models[models.length - 1]?.id).toBe('zai-org/GLM-5.2')
  })

  test('ships the genuine pinned artwork', () => {
    expect(getIconPath('huggingchat')).toBe('/providers/huggingchat.png')
    const png = readFileSync(resolve(root, 'web/public/providers/huggingchat.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBeLessThanOrEqual(512)
    expect(png.readUInt32BE(20)).toBeLessThanOrEqual(512)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
