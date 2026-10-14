<script lang="ts">
  import { apiOrigin } from '$lib/api';
  import { showShell } from '$lib/shell.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Notice from '$lib/ui/Notice.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let message = $state('Runner guide');
  let copied = $state('');

  showShell(() => ({ title: 'Runner guide', message }));

  const origin = apiOrigin.replace(/\/+$/, '');
  const unit = 'orchestrator-runner';

  const commands = {
    install: `curl -fsSL ${origin}/runner/install.sh | sudo bash -s -- --url ${origin} --token <token> --name box-1 --slots 2`,
    check: `systemctl status ${unit}\njournalctl -u ${unit} -f`,
    subids: 'sudo usermod --add-subuids 200000-265535 --add-subgids 200000-265535 orchestrator',
    manual: `orchestrator runner register --url ${origin} --token <token> --name my-laptop --slots 2\norchestrator runner run`,
    source: `cd backend\ngo run ./cmd/orchestrator runner register --url ${origin} --token <token> --name my-laptop --slots 2\ngo run ./cmd/orchestrator runner run`,
    update: `curl -fsSL ${origin}/runner/install.sh | sudo bash -s -- --url ${origin}`,
    uninstall: [
      `sudo systemctl disable --now ${unit}`,
      `sudo rm -rf /etc/systemd/system/${unit}.service /etc/systemd/system/${unit}.service.d`,
      'sudo systemctl daemon-reload',
      'sudo loginctl disable-linger orchestrator',
      'sudo userdel --remove orchestrator',
      'sudo rm -f /usr/local/bin/orchestrator'
    ].join('\n'),
    manualRemove: 'orchestrator runner remove\nrm -rf ~/.local/share/orchestrator-runner'
  };

  async function copy(key: keyof typeof commands) {
    try {
      await navigator.clipboard.writeText(commands[key]);
      copied = key;
      message = 'Command copied';
    } catch {
      message = 'Select the command and copy it.';
    }
  }
</script>

{#snippet command(key: keyof typeof commands, label: string)}
  <div class="relative">
    <pre class="overflow-x-auto rounded-control border border-line bg-ground py-3 pr-12 pl-3 font-mono text-xs whitespace-pre-wrap break-all text-ink">{commands[key]}</pre>
    <button
      type="button"
      class="absolute top-0 right-0 flex size-11 items-center justify-center rounded-control text-muted hover:text-ink"
      aria-label={copied === key ? 'Copied' : label}
      title={copied === key ? 'Copied' : label}
      onclick={() => void copy(key)}
    >
      <Icon name={copied === key ? 'check' : 'copy'} class="size-4" />
    </button>
  </div>
{/snippet}

<Page title="Runner guide" description="How to register a machine as a runner, keep it up to date, pause it, and unregister it.">
  {#snippet actions()}
    <Button variant="ghost" href="/admin/runners">Back to runners</Button>
    <Button variant="primary" href="/admin/runners/new">Add runner</Button>
  {/snippet}
  <div class="flex max-w-3xl flex-col gap-6 text-sm text-ink">
    <Panel title="Before you start">
      <ul class="flex list-disc flex-col gap-2 pl-5">
        <li>A Linux machine, x86_64 or arm64, with systemd, git, curl, and rootless Podman.</li>
        <li>The machine reaches this server over HTTPS. Plain <code class="font-mono text-xs">http://</code> works only for 127.0.0.1 on the same machine.</li>
        <li>
          This server publishes the runner files. An operator runs <code class="font-mono text-xs">scripts/release.sh</code> and starts the server with
          <code class="font-mono text-xs">ORCHESTRATOR_RUNNER_RELEASES</code> set to its output folder. Without that, the install command fails with a 404.
        </li>
        <li>A runner never gets the database password or model keys. It calls this server for every job step.</li>
      </ul>
    </Panel>

    <Panel title="Register a runner" hint="The install script registers the machine and starts it as a service.">
      <ol class="flex list-decimal flex-col gap-4 pl-5">
        <li>
          Open <a class="font-medium text-accent hover:underline" href="/admin/runners/new">Add runner</a>, set a name and the number of slots, and create the install command.
          The token in it works once and expires after one hour.
        </li>
        <li class="flex flex-col gap-2">
          <span>Run the command on the machine. It needs root, so it runs through sudo.</span>
          {@render command('install', 'Copy example')}
        </li>
        <li>Within about 10 seconds the runner shows as Idle on the Runners page.</li>
        <li>To send implement jobs to runners, set <span class="font-medium">Implement jobs run on</span> to Runners on the Runners page.</li>
      </ol>
      <Notice>
        The script installs <code class="font-mono text-xs">/usr/local/bin/orchestrator</code>, creates the <code class="font-mono text-xs">orchestrator</code> user with its home in
        <code class="font-mono text-xs">/var/lib/orchestrator-runner</code>, writes the runner config there readable only by that user, and starts the
        <code class="font-mono text-xs">{unit}</code> service.
      </Notice>
      <div class="flex flex-col gap-2">
        <h3 class="font-medium">Check it on the machine</h3>
        {@render command('check', 'Copy commands')}
      </div>
      <div class="flex flex-col gap-2">
        <h3 class="font-medium">If the script stops on subordinate ids</h3>
        <p class="text-muted">Rootless Podman needs a uid and gid range for the service user. Add one, then run the install command again. The token is still unused at that point.</p>
        {@render command('subids', 'Copy command')}
      </div>
    </Panel>

    <Panel title="Register without the install script" hint="For macOS or a development machine. The runner stops when the terminal closes.">
      <p>Create a token on Add runner, then register once and start the runner.</p>
      {@render command('manual', 'Copy commands')}
      <p class="text-muted">From a checkout of this repository, without an installed binary:</p>
      {@render command('source', 'Copy commands')}
      <p class="text-muted">
        The config is saved in your user config folder: <code class="font-mono text-xs">~/.config/orchestrator/runner.json</code> on Linux,
        <code class="font-mono text-xs">~/Library/Application Support/orchestrator/runner.json</code> on macOS. Later starts need only
        <code class="font-mono text-xs">orchestrator runner run</code>.
      </p>
    </Panel>

    <Panel title="Update a runner">
      <p>Run the install script again without a token. It replaces the binary and the service, keeps the registration, and restarts the runner.</p>
      {@render command('update', 'Copy command')}
      <p class="text-muted">A runner marked Update required is on an older version than this server and gets no new jobs until it is updated.</p>
    </Panel>

    <Panel title="Pause a runner">
      <p>
        Choose <span class="font-medium">Drain</span> on the Runners page. The runner finishes the jobs it has and takes no new ones. Choose
        <span class="font-medium">Resume</span> to give it jobs again. Drain a runner before you restart or patch its machine.
      </p>
    </Panel>

    <Panel title="Unregister a runner" hint="Remove it on this server first, then clean up the machine.">
      <ol class="flex list-decimal flex-col gap-4 pl-5">
        <li>Optional: choose <span class="font-medium">Drain</span> and wait until its slots show 0 busy, so no job is cut off.</li>
        <li>
          Choose <span class="font-medium">Remove</span> and confirm. Its secret stops working at once, and any job it still holds goes back to the queue. The
          service stops itself and is not restarted.
        </li>
        <li class="flex flex-col gap-2">
          <span>On the machine, remove the service, the user, and the binary.</span>
          {@render command('uninstall', 'Copy commands')}
          <span class="text-muted">
            <code class="font-mono text-xs">userdel --remove</code> deletes <code class="font-mono text-xs">/var/lib/orchestrator-runner</code> with the config, clones, worktrees, and the
            user's Podman images. If it reports that the user is still in use, wait a few seconds and run it again.
          </span>
        </li>
      </ol>
      <div class="flex flex-col gap-2">
        <h3 class="font-medium">Registered without the install script</h3>
        <p class="text-muted">Stop the runner with Ctrl-C, remove it on the Runners page, then delete its config and work folder.</p>
        {@render command('manualRemove', 'Copy commands')}
      </div>
      <Notice tone="warning">Cleaning up only the machine leaves the runner listed as Offline, and its secret stays valid. Always remove it on the Runners page.</Notice>
    </Panel>
  </div>
</Page>
