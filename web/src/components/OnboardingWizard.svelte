<!-- 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id -->
<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { onMount } from 'svelte'
  import { Loader2, CheckCircle2, Circle, Stethoscope, X, AlertTriangle, ShieldCheck } from 'lucide-svelte'
  import { api } from '../api/client'

  interface OnboardingStatus {
    hasConnection: boolean
    activeConns: number
    hasKey: boolean
    activeKeys: number
    loginRequired: boolean
    dismissed: boolean
    complete: boolean
  }

  interface DoctorCheck {
    name: string
    status: string
    detail?: string
    fixHint?: string
  }

  let status = $state<OnboardingStatus | null>(null)
  let loadError = $state('')
  let checks = $state<DoctorCheck[] | null>(null)
  let runningDoctor = $state(false)
  let finishing = $state(false)

  onMount(async () => {
    try {
      status = await api.getOnboardingStatus()
    } catch (e) {
      loadError = e instanceof Error ? e.message : String(e)
    }
  })

  async function runDoctor() {
    try {
      runningDoctor = true
      const res = await api.getDoctor()
      checks = res.checks
    } catch (e) {
      loadError = e instanceof Error ? e.message : String(e)
    } finally {
      runningDoctor = false
    }
  }

  async function finish() {
    try {
      finishing = true
      await api.completeOnboarding()
    } catch {
      // The dismissal is cosmetic: a failed write leaves the card for the
      // next visit instead of lying about being done.
    } finally {
      finishing = false
      status = null
    }
  }
</script>

{#if status && !status.dismissed}
<div class="rounded-xl border border-brand-500/30 bg-brand-500/5 p-4 mb-6">
  <div class="flex items-center gap-2">
    <ShieldCheck class="w-4 h-4 text-brand-500" aria-hidden="true" />
    <h2 class="text-sm font-bold">{t('Set up your gateway')}</h2>
    <button
      type="button"
      class="ml-auto rounded p-1 text-text-muted hover:text-text-main transition-colors"
      aria-label={t('Dismiss the setup tour')}
      onclick={finish}
    >
      <X class="w-4 h-4" />
    </button>
  </div>

  {#if loadError}
    <p class="mt-2 text-xs text-red-500" role="alert">{loadError}</p>
  {:else if status}
    <ul class="mt-3 space-y-1.5">
      <li class="flex items-center gap-2 text-sm">
        {#if status.hasConnection}
          <CheckCircle2 class="w-4 h-4 text-emerald-500" aria-hidden="true" />
          <span>Connect a provider ({status.activeConns} active)</span>
        {:else}
          <Circle class="w-4 h-4 text-text-muted" aria-hidden="true" />
          <span>{t('Add a provider in')} <b>{t('Connections')}</b> {t('— that is what traffic routes through')}</span>
        {/if}
      </li>
      <li class="flex items-center gap-2 text-sm">
        {#if status.hasKey}
          <CheckCircle2 class="w-4 h-4 text-emerald-500" aria-hidden="true" />
          <span>Create an API key ({status.activeKeys} active)</span>
        {:else}
          <Circle class="w-4 h-4 text-text-muted" aria-hidden="true" />
          <span>{t('Create an')} <b>{t('API Key')}</b> {t('— that is what your clients call with')}</span>
        {/if}
      </li>
      <li class="flex items-center gap-2 text-sm">
        {#if status.loginRequired}
          <CheckCircle2 class="w-4 h-4 text-emerald-500" aria-hidden="true" />
          <span>{t('Dashboard login armed')}</span>
        {:else}
          <AlertTriangle class="w-4 h-4 text-amber-500" aria-hidden="true" />
          <span>{t('Dashboard login is off — arm it in')} <b>{t('Profile')}</b> {t('if this gateway is reachable beyond loopback')}</span>
        {/if}
      </li>
    </ul>

    <div class="mt-3 flex items-center gap-2">
      <button
        type="button"
        onclick={runDoctor}
        disabled={runningDoctor}
        class="inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-xs font-medium text-text-main hover:bg-surface-2 disabled:opacity-50 transition-colors"
      >
        {#if runningDoctor}
          <Loader2 class="w-3.5 h-3.5 animate-spin" aria-hidden="true" />
        {:else}
          <Stethoscope class="w-3.5 h-3.5" aria-hidden="true" />
        {/if}
        Run health check
      </button>
      <button
        type="button"
        onclick={finish}
        disabled={finishing}
        class="rounded-lg bg-brand-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-brand-500 disabled:opacity-50 transition-colors"
      >{t('Got it — don\'t ask again')}</button>
    </div>

    {#if checks}
      <div class="mt-3 rounded-lg border border-border bg-surface p-3 space-y-1">
        {#each checks as c (c.name)}
          <div class="flex items-center gap-2 text-xs">
            {#if c.status === 'pass'}
              <CheckCircle2 class="w-3.5 h-3.5 text-emerald-500" aria-hidden="true" />
            {:else if c.status === 'warn'}
              <AlertTriangle class="w-3.5 h-3.5 text-amber-500" aria-hidden="true" />
            {:else}
              <AlertTriangle class="w-3.5 h-3.5 text-red-500" aria-hidden="true" />
            {/if}
            <span class="font-medium">{c.name}</span>
            <span class="text-text-muted">{c.detail}</span>
            {#if c.fixHint}
              <span class="text-text-muted italic">— {c.fixHint}</span>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  {:else}
    <div class="mt-3 flex items-center gap-2 text-xs text-text-muted">
      <Loader2 class="w-3.5 h-3.5 animate-spin" aria-hidden="true" />{t('Reading the install state…')}</div>
  {/if}
</div>
{/if}
