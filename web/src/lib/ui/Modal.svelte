<!-- 9RouterX — source port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id -->
<script lang="ts">
  import { t } from '../../lib/i18n.svelte'
  // Port of decolua/9router src/shared/components/Modal.js
  import type { Snippet } from 'svelte'
  import { pushEscapeHandler } from './escapeStack'

  let {
    isOpen = false,
    onClose,
    title = '',
    icon = '',
    size = 'md',
    closeOnOverlay = true,
    showTrafficLights = true,
    class: klass = '',
    children,
    footer
  }: {
    isOpen?: boolean
    onClose?: () => void
    title?: string
    icon?: string
    size?: 'sm' | 'md' | 'lg' | 'xl' | 'full'
    closeOnOverlay?: boolean
    showTrafficLights?: boolean
    class?: string
    children?: Snippet
    footer?: Snippet
  } = $props()

  const sizes = {
    sm: 'max-w-sm',
    md: 'max-w-md',
    lg: 'max-w-lg',
    xl: 'max-w-xl',
    full: 'max-w-4xl'
  }

  // Lock background scroll while the modal is open.
  $effect(() => {
    if (!isOpen) return
    const previous = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = previous
    }
  })

  // Escape closes only the topmost dialog, so a dialog opened over this one
  // (a picker, a confirm) is dismissed without taking this one with it.
  $effect(() => {
    if (!isOpen) return
    return pushEscapeHandler(() => onClose?.())
  })

  // Focus management (C4): the panel takes focus on open and Tab cycles
  // inside it — without the trap, Tab leaves the dialog and the operator's
  // next keystrokes land on the page behind the scrim.
  let panelEl: HTMLElement | undefined = $state()
  $effect(() => {
    if (!isOpen || !panelEl) return
    const previous = document.activeElement as HTMLElement | null
    panelEl.focus()
    return () => previous?.focus?.()
  })
  function trapTab(event: KeyboardEvent) {
    if (event.key !== 'Tab' || !panelEl) return
    const focusables = Array.from(
      panelEl.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
      ),
    ).filter((el) => el.offsetParent !== null || el === document.activeElement)
    if (focusables.length === 0) {
      event.preventDefault()
      panelEl.focus()
      return
    }
    const first = focusables[0]
    const last = focusables[focusables.length - 1]
    const active = document.activeElement
    if (event.shiftKey) {
      if (active === first || !panelEl.contains(active)) {
        event.preventDefault()
        last.focus()
      }
    } else if (active === last) {
      event.preventDefault()
      first.focus()
    }
  }
</script>

{#if isOpen}
  <div class="glyph-modal-layer fixed inset-0 z-50 flex items-center justify-center p-4">
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="absolute inset-0 bg-[var(--app-modal-scrim)] backdrop-blur-[2px] fade-in"
      onclick={closeOnOverlay ? () => onClose?.() : undefined}
      onkeydown={(e) => e.key === 'Escape' && onClose?.()}
      role="presentation"
    ></div>

    <div
      bind:this={panelEl}
      role="dialog"
      aria-modal="true"
      tabindex="-1"
      onkeydown={trapTab}
      class="glyph-modal relative w-full bg-surface border border-border-subtle rounded-[14px] shadow-[var(--shadow-elev)] fade-in {sizes[size]} {klass}"
    >
      {#if title || showTrafficLights}
        <div class="flex items-center justify-between p-2 border-b border-border-subtle">
          <div class="flex items-center">
            {#if showTrafficLights}
              <div class="hidden md:flex items-center gap-2 mr-4 ml-2">
                <button
                  type="button"
                  onclick={() => onClose?.()}
                  aria-label={t('Close')}
                  title={t('Close')}
                  class="w-4 h-4 rounded-full bg-[#FF5F56] hover:brightness-90 transition-all cursor-pointer flex items-center justify-center"
                >
                  <span class="text-[9px] font-bold text-white leading-none">✕</span>
                </button>
                <div class="w-4 h-4 rounded-full bg-surface-3 cursor-not-allowed"></div>
                <div class="w-4 h-4 rounded-full bg-surface-3 cursor-not-allowed"></div>
              </div>
            {/if}
            {#if title}
              <h2 class="flex items-center gap-2 text-lg font-semibold text-text-main">
                {#if icon}
                  <span class="material-symbols-outlined text-[20px] text-brand-500">{icon}</span>
                {/if}
                {title}
              </h2>
            {/if}
          </div>
          <button
            type="button"
            onclick={() => onClose?.()}
            aria-label={t('Close')}
            class="md:hidden p-1.5 rounded-[10px] text-text-muted hover:bg-surface-2 hover:text-text-main transition-colors"
          >
            <span class="material-symbols-outlined text-[18px]">close</span>
          </button>
        </div>
      {/if}

      <div class="glyph-modal-body p-6 max-h-[calc(85vh-100px)] overflow-y-auto custom-scrollbar">
        {@render children?.()}
      </div>

      {#if footer}
        <div class="flex items-center justify-end gap-3 p-6 border-t border-border-subtle">
          {@render footer()}
        </div>
      {/if}
    </div>
  </div>
{/if}
