import { describe, expect, test } from 'bun:test'
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { getModelsByProviderId, PROVIDER_ID_TO_ALIAS } from './models'
import { __setProviderCatalogForTest, PROVIDER_CATALOG } from './providers'
import { parseProviderRegistry } from './providers.registry-test'
import { getIconPath } from '../components/connections/types'

const root = resolve(import.meta.dirname, '../../..')
const fixture = JSON.parse(readFileSync(resolve(root, 'internal/providers/testdata/p1-moonshot.json'), 'utf8'))
__setProviderCatalogForTest(parseProviderRegistry(readFileSync(resolve(root, 'internal/providers/provider_registry.go'), 'utf8')))

describe('Moonshot pinned metadata', () => {
  test('keeps the six Moonshot models and isolated API-key brand', () => {
    expect(fixture.sha).toBe('632d84f569f72893ed148e5b19dd05e2fff1a52a')
    expect(PROVIDER_ID_TO_ALIAS.moonshot).toBe('moonshot')
    expect(getModelsByProviderId('moonshot').map((model) => model.id)).toEqual(fixture.models)
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'moonshot')
    expect(provider?.name).toBe('Kimi')
    expect(provider?.category).toBe('apikey')
    expect(provider?.alias).toBe('moonshot')
    expect(provider?.authModes).toBeUndefined()
    expect(provider?.serviceKinds).toEqual(['llm'])
    expect(PROVIDER_CATALOG.some((entry) => entry.id === 'dify')).toBe(false)
  })

  test('serves the existing Kimi mark through a brand-only alias', () => {
    expect(getIconPath('moonshot')).toBe('/providers/kimi.png')
    const png = readFileSync(resolve(root, fixture.artwork.served))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
    if (fixture.artwork.sha256) expect(createHash('sha256').update(png).digest('hex')).toBe(fixture.artwork.sha256)
  })
})
