<script lang="ts">
  import { ApiError, testPush, unreachable } from '$lib/api';
  import { disablePush, enablePush, syncPush, type PushState } from '$lib/push';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let mode = $state<PushState | 'loading'>('loading');
  let busy = $state(false);
  let result = $state('');

  const hints: Record<PushState | 'loading', string> = {
    loading: 'Checking this browser.',
    unsupported: 'This browser cannot show notifications from the orchestrator.',
    blocked: 'Notifications are blocked for this site. Allow them in your browser’s site settings, then reload this page.',
    off: 'Get a notification on this device when a card needs you, even with the orchestrator closed. It shows the card title and project only.',
    on: 'This browser shows a notification when a card needs you. It shows the card title and project only.'
  };

  $effect(() => {
    void syncPush()
      .then((next) => (mode = next))
      .catch(() => (mode = 'off'));
  });

  async function run(action: () => Promise<PushState>, done: string) {
    busy = true;
    result = '';
    try {
      mode = await action();
      result = mode === 'blocked' ? 'Notifications are blocked in browser settings.' : done;
    } catch (error) {
      result = error instanceof ApiError ? error.message : error instanceof Error ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  async function sendTest() {
    busy = true;
    result = '';
    try {
      const { sent } = await testPush();
      result = sent > 0 ? 'Test sent. It should appear in a few seconds.' : 'No browser took the test. Turn notifications off and on again.';
    } catch (error) {
      result = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Panel title="Browser notifications" hint={hints[mode]}>
  {#snippet actions()}
    {#if mode === 'on'}
      <Badge tone="green">On</Badge>
    {:else if mode === 'blocked'}
      <Badge tone="amber">Blocked in browser settings</Badge>
    {:else if mode === 'off'}
      <Badge>Off</Badge>
    {/if}
  {/snippet}
  <p class="text-sm text-muted" role="status">{result}</p>
  {#snippet footer()}
    {#if mode === 'on'}
      <Button variant="secondary" disabled={busy} onclick={() => void sendTest()}>Send a test</Button>
      <Button variant="secondary" disabled={busy} onclick={() => void run(disablePush, 'Notifications are off for this browser.')}>
        Turn off
      </Button>
    {:else if mode === 'off'}
      <Button variant="primary" disabled={busy} onclick={() => void run(enablePush, 'Notifications are on for this browser.')}>
        Turn on
      </Button>
    {/if}
  {/snippet}
</Panel>
