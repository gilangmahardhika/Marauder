<script lang="ts">
  import type { Intake } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';

  let {
    intake,
    busy = false,
    oningest,
    onaccept
  }: {
    intake: Intake | null;
    busy?: boolean;
    oningest: () => void;
    onaccept: () => void;
  } = $props();

  const canAccept = $derived(Boolean(intake?.canAccept));
</script>

<Button type="button" variant="secondary" disabled={busy} onclick={oningest}>Ingest</Button>
<Button variant={canAccept ? 'secondary' : 'primary'} href="/intake/new" disabled={busy}>Add requirement</Button>
{#if canAccept}
  <Button type="button" variant="primary" disabled={busy || Boolean(intake?.draftPending)} onclick={onaccept}>
    Accept unaccepted ids
  </Button>
{/if}
