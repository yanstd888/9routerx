### Added
- **Simplified Chinese (`zh`) joins the dashboard locale set.** `web/src/locales/zh.json`
  carries all **1,456** keys the existing catalogue defines, so every string that
  already renders through `t()` switches over — nothing is left falling back to
  English on a locale that claims to be complete.

  The catalogue is keyed by the English source text, which is what made this a
  one-file addition: no component had to change, and a key that is missing from
  another language still falls back safely.

  Two supporting edits in `web/src/lib/i18n.svelte.ts`: `Locale` gains `'zh'`,
  `SUPPORTED_LOCALES` gains the `简体中文 / 🇨🇳` entry, and the two hand-written
  `saved === 'en' || saved === 'id'` checks became an `isLocale` guard over a
  `LOCALES` list. Without that guard, adding a third locale meant remembering to
  widen three separate comparisons, and a stored `zh` would have been silently
  discarded on the next load.

- The settings hint read `(id / en / zh / ja)` while only two locales existed.
  It now reads `(en / id / zh)`: `ja` has no catalogue, and the order matches
  `SUPPORTED_LOCALES` rather than the order they were written in.

### Not in this change

`ja` remains untranslated. Roughly 1,300 rendered strings still have no `t()`
wrapper at all — the catalogue is complete, but only 71 call sites are wired
today, so switching to Chinese translates those 71 and leaves the rest in
English. Extending the wrappers is the next batch.

### Verification

- `bun run build` — success; 2,630 KB pre-compressed to 618 KB brotli.
  `简体中文` and the translated button labels are present in
  `dist/assets/index-*.js`.
- `bun run ratchet:svelte` — **0 unresolved identifiers, 71 errors (baseline 71)**.
  Neither `i18n.svelte.ts` nor `zh.json` contributes a new diagnostic.
- Key parity checked mechanically against `id.json`: 1,456 expected, 1,456
  present, 0 missing, 0 extra, 0 duplicate. Every `{placeholder}` in a key also
  appears in its translation (0 mismatches).
- 39 values are deliberately identical to their key — brand and protocol names
  (`Claude`, `GitHub`, `OAuth`, `MITM`, `SSE URL`, `npx 9router`, the
  `kiro://…` callback example). They are not mistranslations and are expected to
  read the same in both languages.
