<script lang="ts">
  import Button from './Button.svelte';
  import type { IconName } from './icons';

  let {
    busy = false,
    disabled = false,
    label = 'Save',
    icon = 'save',
    note = ''
  }: { busy?: boolean; disabled?: boolean; label?: string; icon?: IconName; note?: string } = $props();
</script>

<div class="sticky bottom-0 z-10 mt-8 flex items-center justify-between gap-4 border-t border-line bg-ground py-4">
  <p class="min-w-0 text-sm text-muted">{note}</p>
  <Button variant="primary" type="submit" {icon} loading={busy} {disabled}>{label}</Button>
</div>
