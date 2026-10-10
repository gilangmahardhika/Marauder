<script lang="ts">
  import { page } from '$app/stores';
  import { showShell } from '$lib/shell.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';

  showShell(() => ({ title: String($page.status), message: '' }));
</script>

<Page title="Something went wrong" description={`Error ${$page.status}`}>
  <EmptyState title={$page.error?.message ?? 'Unknown error'} href="/" link="Back to the board" />
</Page>
