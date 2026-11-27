<!-- 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id -->
<script lang="ts">
  import { t } from '../../lib/i18n.svelte'
  import { onMount } from 'svelte'
  import Card from '../../lib/ui/Card.svelte'
  import Button from '../../lib/ui/Button.svelte'
  import { RefreshCw } from 'lucide-svelte'
  import { currentCurrency } from '../../lib/currency.svelte'
  import { rateFor, type DisplayCurrency } from '../../lib/currency'

  // Savings Ledger (C2): one honest USD figure for the window, converted to
  // the operator's display currency with the manually-set rate. No live FX
  // fetch — the gateway must stay correct offline.
  interface SavingsSource {
    source: string
    tokensSaved: number
    usd: number
    note?: string
    top5?: Array<{ provider?: string; model?: string; tokensSaved: number; usd: number; cachedTokens?: number }>
  }
  interface SavingsSummary {
    window: string
    totalTokensSaved: number
    totalUsd: number
    sources: SavingsSource[]
  }

  const WINDOWS = ['7d', '30d', 'all'] as const
  let window = $state<'7d' | '30d' | 'all'>('7d')
  let summary = $state<SavingsSummary | null>(null)
  let loading = $state(false)
  let failed = $state(false)

  const sourceLabel: Record<string, string> = {
    prompt_cache: 'Prompt cache',
    compression: 'RTK / GCF compression',
    token_saver: 'Token saver',
  }

  async function load() {
    loading = true
    failed = false
    try {
      const res = await fetch(`/api/savings/summary?period=${window}`)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      summary = await res.json()
    } catch {
      failed = true
    } finally {
      loading = false
    }
  }

  onMount(load)

  function displayUsd(usd: number): string {
    const cur: DisplayCurrency = currentCurrency()
    const rate = rateFor(cur.code, cur) ?? 1
    const converted = usd * rate
    if (converted === 0) return cur.code === 'USD' ? '$0.00' : `0.00 ${cur.code}`
    return `${converted.toLocaleString('id-ID', { maximumFractionDigits: 2 })} ${cur.code}`
  }
</script>

<Card padding="md" class="space-y-3">
  <div class="flex items-center justify-between pb-2 border-b border-border">
    <div class="flex items-center gap-2">
      <h3 class="text-sm font-semibold text-text-main">{t('Savings Ledger')}</h3>
      <span class="text-[11px] text-text-subtle">
        {summary ? `${summary.totalTokensSaved.toLocaleString()} tokens saved` : ''}
      </span>
    </div>
    <div class="flex items-center gap-1.5">
      {#each WINDOWS as w (w)}
        <Button
          variant={window === w ? 'primary' : 'ghost'}
          size="sm"
          onclick={() => { window = w; load(); }}
        >
          {w}
        </Button>
      {/each}
      <Button variant="ghost" size="sm" onclick={load} loading={loading}>
        <RefreshCw class="w-3.5 h-3.5 text-text-muted" />
      </Button>
    </div>
  </div>

  {#if failed}
    <p class="text-xs text-text-muted">{t('Savings summary unavailable right now.')}</p>
  {:else if summary}
    <div class="rounded-[10px] bg-bg border border-border p-3">
      <p class="text-[10px] uppercase tracking-wider text-text-subtle font-mono font-semibold">
        This {summary.window} · {displayUsd(summary.totalUsd)}
      </p>
      <p class="text-2xl font-bold font-code text-emerald-500 mt-1">
        ${summary.totalUsd.toFixed(2)}
      </p>
    </div>
    <table class="glyph-table w-full text-left text-xs">
      <thead class="text-text-muted">
        <tr class="border-b border-border-subtle">
          <th scope="col" class="py-2 px-3 font-medium">{t('Source')}</th>
          <th scope="col" class="py-2 px-3 font-medium text-right">{t('Tokens saved')}</th>
          <th scope="col" class="py-2 px-3 font-medium text-right">{t('USD')}</th>
        </tr>
      </thead>
      <tbody class="divide-y divide-border-subtle">
        {#each summary.sources as s (s.source)}
          <tr class="hover:bg-surface-2/60 transition-colors">
            <td class="py-2 px-3">
              {sourceLabel[s.source] ?? s.source}
              {#if s.note}<span class="text-text-subtle"> — {s.note}</span>{/if}
            </td>
            <td class="py-2 px-3 text-right font-mono">{s.tokensSaved.toLocaleString()}</td>
            <td class="py-2 px-3 text-right font-mono text-emerald-500">
              ${s.usd > 0 ? s.usd.toFixed(2) : '0.00'}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
    {#if summary.sources.some((s) => s.top5 && s.top5.length > 0)}
      <details class="text-xs">
        <summary class="cursor-pointer text-text-muted hover:text-text-main transition-colors">{t('Top 5 breakdown')}</summary>
        {#each summary.sources.filter((s) => s.top5?.length) as s (s.source)}
          <p class="mt-2 font-semibold text-text-main">{sourceLabel[s.source] ?? s.source}</p>
          {#each s.top5 ?? [] as row (row.provider ?? row.model)}
            <p class="text-text-muted">
              {row.provider ?? row.model}: {row.tokensSaved.toLocaleString()} tokens
              {#if row.usd > 0}· ${row.usd.toFixed(2)}{/if}
            </p>
          {/each}
        {/each}
      </details>
    {/if}
  {:else}
    <p class="py-6 text-center text-xs text-text-muted">{t('Loading savings…')}</p>
  {/if}
</Card>
