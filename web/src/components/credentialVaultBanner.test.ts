// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
// The credential-encryption banner is the only surface that tells an operator
// their credentials are being written unencrypted, and after the
// ROUTER_MASTER_KEY docs landed its "How to set it" link is the only
// discoverable path to the fix. This pins the contract that the follow-up E2E
// (still to be repaired separately) depends on:
//   - the banner renders whenever the vault status read succeeds and reports
//     enabled=false, with no sealed-credential-count gating;
//   - the link targets the README's credential-encryption anchor, which the
//     docs commit ships, with a safe rel for an external target;
//   - App mounts the banner unconditionally, so no tab or route can hide it.
// The repo has no DOM component-test harness (bun test units are logic-only,
// and adding a DOM runner would add dependencies), so the contract is pinned
// against the compiled-from source the way providerPrivacyLabels.test.ts does.
import { readFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { describe, expect, it } from 'bun:test'

const component = readFileSync(
  join(import.meta.dirname, 'CredentialVaultBanner.svelte'),
  'utf8',
)
const app = readFileSync(resolve(import.meta.dirname, '..', 'App.svelte'), 'utf8')
const readme = readFileSync(resolve(import.meta.dirname, '..', '..', '..', 'README.md'), 'utf8')
const zhCatalogue = JSON.parse(
  readFileSync(resolve(import.meta.dirname, '..', 'locales', 'zh.json'), 'utf8'),
) as Record<string, string>

describe('credential encryption banner contract', () => {
  it('renders when the vault status read succeeds and reports disabled', () => {
    expect(component).toContain('{#if status && !status.enabled}')
    expect(component).toContain('status = await api.getVaultStatus()')
    // A failed read must not become a fake warning: status stays null and the
    // {#if} above renders nothing.
    expect(component).toContain('status = null')
  })

  it('does not gate rendering on sealed-credential counts', () => {
    expect(component).not.toContain('sealedCount')
    expect(component).not.toContain('plaintextCount')
  })

  it('links "How to set it" to the README credential-encryption anchor', () => {
    const anchor = 'https://yanstd888.github.io/9routerx/readme.html#credential-encryption'
    expect(component).toContain(`href="${anchor}"`)
    expect(component).toContain('rel="noopener noreferrer"')
    // The label goes through t() now, so the English text is the dictionary key
    // rather than literal markup. Pin the wrapper, and pin that the key exists —
    // a typo in either half would silently fall back to English.
    expect(component).toContain(">{t('How to set it')}</a>")
    expect(zhCatalogue['How to set it']).toBe('如何设置')
    // The anchor only resolves once the docs section exists; pin both halves.
    expect(readme).toContain('## Credential encryption')
    expect(readme).toContain('ROUTER_MASTER_KEY')
  })

  it('is announced as an alert and mounted unconditionally in the shell', () => {
    expect(component).toContain('role="alert"')
    expect(app).toContain('<CredentialVaultBanner />')
    // The mount sits at the shell level, not behind a tab condition: the line
    // before it opens a plain wrapper div, not an {#if}.
    const at = app.indexOf('<CredentialVaultBanner />')
    expect(app.slice(at - 120, at)).toContain('class="px-6 lg:px-10 pt-4"')
  })
})
