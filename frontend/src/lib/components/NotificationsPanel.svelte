<script lang="ts">
  import { testPush } from '$lib/api';
  import { failureText } from '$lib/forms';
  import { disablePush, enablePush, syncPush, type PushState } from '$lib/push';
  import { toast } from '$lib/toast.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let mode = $state<PushState | 'loading'>('loading');
  let pending = $state<'' | 'on' | 'off' | 'test'>('');

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

  async function run(which: 'on' | 'off', action: () => Promise<PushState>, done: string) {
    pending = which;
    try {
      mode = await action();
      if (mode === 'blocked') toast.error('Notifications are blocked in browser settings.');
      else toast.success(done);
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      pending = '';
    }
  }

  async function sendTest() {
    pending = 'test';
    try {
      const { sent } = await testPush();
      if (sent > 0) toast.success('Test sent. It should appear in a few seconds.');
      else toast.error('No browser took the test. Turn notifications off and on again.');
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      pending = '';
    }
  }
</script>

<Panel title="Browser notifications">
  {#snippet actions()}
    {#if mode === 'on'}
      <Badge tone="green">On</Badge>
    {:else if mode === 'blocked'}
      <Badge tone="amber">Blocked in browser settings</Badge>
    {:else if mode === 'off'}
      <Badge>Off</Badge>
    {/if}
  {/snippet}
  <p class="text-sm text-muted">{hints[mode]}</p>
  {#snippet footer()}
    {#if mode === 'on'}
      <Button
        variant="secondary"
        icon="send"
        loading={pending === 'test'}
        disabled={pending !== ''}
        onclick={() => void sendTest()}>Send a test</Button
      >
      <Button
        variant="secondary"
        icon="bell-off"
        loading={pending === 'off'}
        disabled={pending !== ''}
        onclick={() => void run('off', disablePush, 'Notifications are off for this browser.')}>Turn off</Button
      >
    {:else if mode === 'off'}
      <Button
        variant="primary"
        icon="bell"
        loading={pending === 'on'}
        disabled={pending !== ''}
        onclick={() => void run('on', enablePush, 'Notifications are on for this browser.')}>Turn on</Button
      >
    {/if}
  {/snippet}
</Panel>
