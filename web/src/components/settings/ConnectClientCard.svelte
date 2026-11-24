<!-- 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id -->
<script lang="ts">
  import { t } from '../../lib/i18n.svelte'
  import Card from '../../lib/ui/Card.svelte'
  import Button from '../../lib/ui/Button.svelte'
  import { notifications } from '../../lib/notifications'

  // The dashboard drives the same clientconfig patcher the `9routerx connect`
  // CLI uses — endpoint, credential and model only; the server-side guard
  // refuses a hooks-shaped payload before anything is written.
  const CLIENTS = [
    'claude-code', 'codex', 'opencode', 'droid', 'crush', 'kilo', 'cline', 'pi', 'omp',
  ]

  let client = $state('claude-code')
  let baseUrl = $state('http://127.0.0.1:20128')
  let apiKey = $state('')
  let busy = $state(false)
  let lastDiff = $state<string[]>([])

  async function run(dryRun: boolean) {
    if (!apiKey.trim()) {
      notifications.error('A gateway API key is required — create one in Endpoint & Key first.')
      return
    }
    busy = true
    lastDiff = []
    try {
      const res = await fetch('/api/cli-tools/connect', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ client, baseUrl, apiKey, dryRun }),
      })
      const data = await res.json().catch(() => ({}))
      if (!res.ok) {
        notifications.error(data?.error?.message || `Connect failed (HTTP ${res.status}).`)
        return
      }
      if (dryRun) {
        lastDiff = (data.edits ?? []).map(
          (e: { file: string; keyPath: string; value: string }) => `${e.file}: ${e.keyPath} = ${e.value}`,
        )
        notifications.success('Dry run complete — no file was written.')
      } else {
        notifications.success(`Client config updated${data.backups?.length ? ` (backups: ${data.backups.join(', ')})` : ''}.`)
      }
    } catch (err) {
      notifications.error(err instanceof Error ? err.message : 'Connect failed.')
    } finally {
      busy = false
    }
  }
</script>

<Card padding="md" class="space-y-3">
  <div class="pb-2 border-b border-border">
    <h2 class="text-sm font-bold text-text-main">{t('Connect an AI client')}</h2>
    <p class="text-[11px] text-text-subtle mt-0.5">{t('Writes the gateway endpoint and credential into the client\'s config file — nothing else, ever.')}</p>
  </div>
  <div class="grid grid-cols-1 sm:grid-cols-3 gap-2">
    <label class="text-xs text-text-muted">{t('Client')}<select bind:value={client} class="mt-1 w-full rounded-[10px] border border-border-subtle bg-bg px-2 py-1.5 text-xs text-text-main">
        {#each CLIENTS as c (c)}
          <option value={c}>{c}</option>
        {/each}
      </select>
    </label>
    <label class="text-xs text-text-muted sm:col-span-2">{t('Gateway base URL')}<input bind:value={baseUrl} class="mt-1 w-full rounded-[10px] border border-border-subtle bg-bg px-2 py-1.5 text-xs font-mono text-text-main" />
    </label>
    <label class="text-xs text-text-muted sm:col-span-3">{t('Gateway API key')}<input type="password" bind:value={apiKey} placeholder={t('sk-...')} class="mt-1 w-full rounded-[10px] border border-border-subtle bg-bg px-2 py-1.5 text-xs font-mono text-text-main" />
    </label>
  </div>
  <div class="flex flex-wrap gap-2">
    <Button variant="secondary" size="sm" onclick={() => run(true)} disabled={busy}>{t('Dry run')}</Button>
    <Button variant="primary" size="sm" onclick={() => run(false)} disabled={busy}>{t('Write config')}</Button>
  </div>
  {#if lastDiff.length > 0}
    <pre class="rounded-[10px] bg-bg border border-border p-2 text-[10px] font-mono text-text-muted overflow-x-auto">{lastDiff.join('\n')}</pre>
  {/if}
</Card>
