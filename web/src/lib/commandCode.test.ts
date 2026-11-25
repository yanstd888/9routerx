// 9RouterX — pinned Command Code metadata, model catalog and glyph.
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { getIconPath, getProviderGlyph } from '../components/connections/types'
import { getModelsByProviderId, PROVIDER_ID_TO_ALIAS } from './models'
import { __setProviderCatalogForTest, PROVIDER_CATALOG } from './providers'
import { parseProviderRegistry } from './providers.registry-test'

const root = resolve(import.meta.dirname, '../../..')
__setProviderCatalogForTest(
  parseProviderRegistry(readFileSync(resolve(root, 'internal/providers/provider_registry.go'), 'utf8')),
)

const modelIds = [
  'claude-opus-4-7',
  'claude-fable-5',
  'claude-opus-4-6',
  'claude-sonnet-5-5',
  'claude-sonnet-4-6',
  'claude-haiku-4-5-20251001',
  'gpt-5.5',
  'gpt-5.4',
  'gpt-5.3-codex',
  'gpt-5.4-mini',
  'deepseek/deepseek-v4-pro',
  'deepseek/deepseek-v4-flash',
  'deepseek/deepseek-v4-flash-vision-exp',
  'moonshotai/Kimi-K3.1-Pro',
  'moonshotai/Kimi-K2.6',
  'moonshotai/Kimi-K2.5',
  'zai-org/GLM-5.1',
  'zai-org/GLM-5',
  'MiniMaxAI/MiniMax-M2.7',
  'MiniMaxAI/MiniMax-M2.5',
  'Qwen/Qwen3.6-Max-Preview',
  'Qwen/Qwen3.6-Plus',
]

// OmniRoute 632d84f5: config/providers/registry/command-code/index.ts and
// src/shared/constants/providers/apikey/gateways.ts. This is the Provider API
// lane, not the legacy `commandcode` CLI /alpha/generate entry.
describe('Command Code pinned metadata', () => {
  test('publishes an isolated Provider API registry entry', () => {
    const provider = PROVIDER_CATALOG.find((entry) => entry.id === 'command-code')
    expect(provider?.name).toBe('Command Code')
    expect(provider?.alias).toBe('cmd')
    expect(provider?.color).toBe('#111827')
    expect(provider?.icon).toBe('terminal')
    expect(provider?.website).toBe('https://commandcode.ai/')
    expect(provider?.authHint).toContain('Bearer token')
    expect(PROVIDER_ID_TO_ALIAS['command-code']).toBe('cmd')
  })

  test('carries the 22 pinned model ids for id and alias', () => {
    expect(getModelsByProviderId('command-code').map((model) => model.id)).toEqual(modelIds)
    expect(getModelsByProviderId('cmd').map((model) => model.id)).toEqual(modelIds)
  })

  test('marks the six Claude-target models', () => {
    const models = getModelsByProviderId('command-code')
    for (const model of models.filter((entry) => entry.id.startsWith('claude-'))) {
      expect(model.supportedFormats).toEqual(['claude'])
    }
    for (const model of models.filter((entry) => !entry.id.startsWith('claude-'))) {
      expect(model.supportedFormats).toEqual(['openai'])
    }
  })

  test('ships the pin-derived 128px CC glyph', () => {
    expect(getIconPath('command-code')).toBe('/providers/command-code.png')
    expect(getIconPath('cmd')).toBe('/providers/command-code.png')
    expect(getProviderGlyph('cmd').name).toBe('Command Code')
    expect(getProviderGlyph('cmd').color).toBe('#111827')
    const png = readFileSync(resolve(root, 'web/public/providers/command-code.png'))
    expect(png.subarray(1, 4).toString()).toBe('PNG')
    expect(png.readUInt32BE(16)).toBe(128)
    expect(png.readUInt32BE(20)).toBe(128)
    expect(png.length).toBeLessThanOrEqual(60 * 1024)
  })
})
