<script lang="ts">
  import { getMyUsage } from '$lib/api';
  import UsageDashboard from '$lib/components/UsageDashboard.svelte';
  import type { Dimension } from '$lib/usage';
  import Button from '$lib/ui/Button.svelte';

  const dimensions: Dimension[] = ['llm', 'model', 'project', 'task', 'source', 'card'];
</script>

<UsageDashboard
  title="My token usage"
  description="Tokens used by model calls on cards you own and by requirements you refined, in every project. Days are in UTC. Total is input plus output; cached input is counted apart."
  load={getMyUsage}
  {dimensions}
>
  {#snippet actions()}
    <Button href="/account">Account</Button>
  {/snippet}
</UsageDashboard>
