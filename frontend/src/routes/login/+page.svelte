<script lang="ts">
  import { goto } from '$app/navigation';
  import { ApiError, openSession } from '$lib/api';
  import Live from '$lib/components/Live.svelte';
  import { session } from '$lib/session';
  import { showShell } from '$lib/shell.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import Icon from '$lib/ui/Icon.svelte';

  let email = $state('');
  let password = $state('');
  let message = $state('');
  let busy = $state(false);

  const facts = [
    'Requirements stay accepted before code.',
    'Each card is one pull request.',
    'Deploy starts only after a person approves a commit.'
  ];

  showShell(() => ({ title: 'Sign in' }));

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    message = 'Signing in';
    try {
      const body = await openSession(email, password);
      session.set(body.bearer);
      await goto('/choose-tribe');
    } catch (error) {
      if (error instanceof ApiError && error.status === 429) message = 'Too many attempts. Wait one minute.';
      else message = error instanceof Error ? error.message : 'API unreachable at http://127.0.0.1:8787';
    } finally {
      busy = false;
    }
  }
</script>

<div class="flex min-h-screen bg-surface">
  <section class="hidden w-[42%] flex-col justify-between bg-accent p-10 text-surface lg:flex">
    <div class="flex items-center gap-2">
      <span class="flex size-7 items-center justify-center rounded-control bg-surface text-xs font-semibold text-accent">M</span>
      <span class="font-semibold">Marauder</span>
    </div>
    <div class="max-w-md">
      <p class="text-3xl font-semibold tracking-tight">Run a repo from accepted requirements to a tested dev deploy.</p>
      <ul class="mt-10 flex flex-col gap-5 text-sm text-surface/80">
        {#each facts as fact (fact)}
          <li class="flex gap-3">
            <Icon name="check" class="mt-0.5 size-4 shrink-0" />
            {fact}
          </li>
        {/each}
      </ul>
    </div>
    <p class="text-xs text-surface/70">Internal engineering workspace</p>
  </section>
  <section class="flex flex-1 items-center justify-center px-6 py-12">
    <div class="w-full max-w-sm rounded-panel border border-line bg-surface p-6 shadow-lg lg:border-0 lg:p-0 lg:shadow-none">
      <div class="mb-8 flex items-center gap-2 lg:hidden">
        <span class="flex size-7 items-center justify-center rounded-control bg-accent text-xs font-semibold text-surface">M</span>
        <span class="font-semibold">Marauder</span>
      </div>
      <h1 class="text-xl font-semibold">Sign in</h1>
      <p class="mt-1 text-sm text-muted">Use the email and password an admin gave you.</p>
      <form class="mt-6 flex flex-col gap-4" onsubmit={submit}>
        <Field label="Email" id="email" type="email" width="full" autocomplete="email" bind:value={email} />
        <Field
          label="Password"
          id="password"
          type="password"
          width="full"
          autocomplete="current-password"
          bind:value={password}
        />
        <Button variant="primary" type="submit" full disabled={busy}>{busy ? 'Signing in' : 'Sign in'}</Button>
      </form>
      <div class="mt-4">
        <Live {message} />
      </div>
    </div>
  </section>
</div>
