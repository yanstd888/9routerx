// 9RouterX — pinned Gitlawb GMI Cloud metadata, catalog and artwork.
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

// OmniRoute 632d84f5: config/providers/registry/gitlawb/gmi/index.ts and
// shared.ts CHAT_OPENAI_COMPAT_MODELS["gitlawb-gmi"]; the chat route was
// verified live on 2026-11-28.
describe('Gitlawb GMI Cloud pinned metadata', () => {
  test('publishes the passthrough gateway metadata', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'gitlawb-gmi')
    expect(provider?.name).toBe('Gitlawb Opengateway (GMI Cloud)')
    expect(provider?.alias).toBe('glb-gmi')
    expect(provider?.color).toBe('#10B981')
    expect(provider?.icon).toBe('hub')
    expect(provider?.website).toBe('https://opengateway.gitlawb.com')
    expect(provider?.authHint).toContain('pay-as-you-go')
    expect(PROVIDER_ID_TO_ALIAS['gitlawb-gmi']).toBe('glb-gmi')
  })

  test('carries the 40 pinned GMI Cloud models', () => {
    const models = getModelsByProviderId('glb-gmi')
    expect(models.length).toBe(40)
    expect(models[0].id).toBe('XiaomiMiMo/MiMo-V2.5-Pro')
    expect(models[models.length - 1].id).toBe('nvidia/NVIDIA-Nemotron-3-Nano-Omni')
  })

  test('ships the shared Gitlawb brand mark at 128px', () => {
    expect(getIconPath('gitlawb-gmi')).toBe('/providers/gitlawb-gmi.png')
    const png = readFileSync(resolve(root, 'web/public/providers/gitlawb-gmi.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
