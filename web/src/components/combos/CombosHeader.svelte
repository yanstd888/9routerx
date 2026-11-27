<!-- 9RouterX — source port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id -->
<script lang="ts">
  import { t } from '../../lib/i18n.svelte'
  import Button from '../../lib/ui/Button.svelte'

  interface Props {
    onCreateClick: () => void
  }

  let { onCreateClick }: Props = $props()
</script>

<div class="flex flex-col gap-4">
  <div class="min-w-0">
    <h1 class="font-headline text-2xl sm:text-3xl font-bold text-text-main tracking-tight">{t('Model Combos')}</h1>
    <p class="text-sm text-text-muted mt-1">{t('Group models under one name, then pick a strategy per combo:')}</p>
    <ul class="text-sm text-text-muted mt-2 flex flex-col gap-1">
      <li>
        <span class="font-medium text-text-main">{t('Fallback')}</span>{t('— tries models in order (next on failure)')}</li>
      <li>
        <span class="font-medium text-text-main">{t('Round Robin')}</span>{t('— rotates models across requests to spread load')}</li>
      <li>
        <span class="font-medium text-text-main">{t('Fusion')}</span>{t('— queries all models in parallel, then a judge synthesizes one answer. Best quality, but costs the most: every request bills all panel models + the judge (N+1 calls)')}</li>
    </ul>
  </div>

  <!-- Upstream parity: the header carries one control. Bulk actions live in the
       selection bar next to the combos, so a destructive button never sits
       permanently beside Create. -->
  <div
    class="flex flex-wrap items-center gap-2 border-t border-border pt-4"
    role="group"
    aria-label={t('Combo actions')}
  >
    <Button icon="add" size="sm" onclick={onCreateClick} class="whitespace-nowrap">{t('Create Combo')}</Button>
  </div>
</div>
