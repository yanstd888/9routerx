// 9RouterX — pinned ChatPlayground metadata and glyph.
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

// OmniRoute 632d84f5: config/providers/registry/chatplayground/index.ts and
// src/shared/constants/providers/web-cookie-chatplayground.ts.
describe('ChatPlayground pinned metadata', () => {
  test('publishes the Clerk-authenticated entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'chatplayground')
    expect(provider?.name).toBe('ChatPlayground')
    expect(provider?.alias).toBe('cpl')
    expect(provider?.color).toBe('#7C3AED')
    expect(provider?.icon).toBe('auto_awesome')
    expect(provider?.website).toBe('https://web.chatplayground.ai')
    expect(provider?.authHint).toContain('Clerk session')
    expect(PROVIDER_ID_TO_ALIAS['chatplayground']).toBe('cpl')
  })

  test('carries the 33 pinned fallback models for id and alias', () => {
    const ids = getModelsByProviderId('chatplayground').map((model) => model.id)
    expect(ids).toHaveLength(33)
    expect(ids[0]).toBe('gpt-6.1-sol')
    expect(ids[ids.length - 1]).toBe('perplexity-sonar')
    expect(getModelsByProviderId('cpl').map((model) => model.id)).toEqual(ids)
  })

  test('ships the pin-derived 128px CP glyph', () => {
    expect(getIconPath('chatplayground')).toBe('/providers/chatplayground.png')
    expect(getIconPath('cpl')).toBe('/providers/chatplayground.png')
    const png = readFileSync(resolve(root, 'web/public/providers/chatplayground.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
