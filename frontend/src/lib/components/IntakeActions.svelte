<script lang="ts">
  import Button from '$lib/ui/Button.svelte';

  let {
    busy = false,
    ingesting = false,
    selecting = false,
    oningest
  }: {
    busy?: boolean;
    /** Ingest is in flight; its button shows the spinner. */
    ingesting?: boolean;
    selecting?: boolean;
    oningest: () => void;
  } = $props();
</script>

<Button type="button" variant="secondary" loading={ingesting} disabled={busy} onclick={oningest}>Ingest</Button>
<Button variant={selecting ? 'secondary' : 'primary'} href="/intake/new" icon="plus" disabled={busy}>Add requirement</Button>
