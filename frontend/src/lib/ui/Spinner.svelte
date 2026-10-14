<script lang="ts">
  let { class: extra = 'size-4' }: { class?: string } = $props();
</script>

<svg class="shrink-0 animate-spin {extra}" viewBox="0 0 24 24" fill="none" aria-hidden="true">
  <circle cx="12" cy="12" r="9" stroke="currentColor" stroke-opacity="0.25" stroke-width="2.5" />
  <path d="M21 12a9 9 0 00-9-9" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" />
</svg>
