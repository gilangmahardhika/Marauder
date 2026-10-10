<script lang="ts">
  import { goto } from '$app/navigation';
  import { ApiError, openSession } from '$lib/api';
  import Live from '$lib/components/Live.svelte';
  import { session } from '$lib/session';

  let token = $state('');
  let message = $state('');
  let busy = $state(false);

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    message = 'Opening session';
    try {
      const body = await openSession(token);
      session.set(body.bearer);
      await goto('/');
    } catch (error) {
      if (error instanceof ApiError && error.status === 429) message = 'Too many attempts. Wait one minute.';
      else message = error instanceof Error ? error.message : 'API unreachable at http://127.0.0.1:8787';
    } finally {
      busy = false;
    }
  }
</script>

<h1>Open session</h1>
<Live {message} />
<form class="form" onsubmit={submit}>
  <label for="token">
    UI token
    <input id="token" type="password" autocomplete="off" bind:value={token} />
  </label>
  <button class="primary" type="submit" disabled={busy}>Open session</button>
</form>
