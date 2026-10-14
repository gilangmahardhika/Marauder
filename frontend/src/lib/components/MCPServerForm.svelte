<script lang="ts" module>
  import type { MCPTransport } from '$lib/types';

  export function isDatabase(transport: MCPTransport): boolean {
    return transport === 'postgres' || transport === 'sqlserver';
  }

  export function defaultPort(transport: MCPTransport): number {
    return transport === 'sqlserver' ? 1433 : 5432;
  }

  /** A slug for a catalog name: the last path part, lowercase letters and digits, starting with a letter. */
  export function suggestSlug(name: string): string {
    const last = name.slice(name.lastIndexOf('/') + 1).toLowerCase();
    const slug = last.replace(/[^a-z0-9]/g, '').replace(/^[0-9]+/, '').slice(0, 16);
    return slug.length >= 2 ? slug : 'mcp' + slug;
  }

  const pinned = /@sha256:[0-9a-f]{64}$/;
</script>

<script lang="ts">
  import { goto } from '$app/navigation';
  import { createMCPServer, deleteMCPServer, discoverMCPTools, pullMCPImage, putMCPServer } from '$lib/api';
  import SecretField from '$lib/components/SecretField.svelte';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { toast } from '$lib/toast.svelte';
  import type { MCPCatalogEntry, MCPDBConfig, MCPDiscoveredTool, MCPServer, MCPServerInput, MCPTool } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Checkbox from '$lib/ui/Checkbox.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Field from '$lib/ui/Field.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Notice from '$lib/ui/Notice.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Select from '$lib/ui/Select.svelte';
  import Table from '$lib/ui/Table.svelte';

  type SecretRow = { name: string; value: string; clear: boolean; saved: boolean; hint?: string; required?: boolean };

  let {
    tribeId,
    server = null,
    entry = null,
    autoDiscover = false
  }: {
    tribeId: string;
    server?: MCPServer | null;
    entry?: MCPCatalogEntry | null;
    autoDiscover?: boolean;
  } = $props();

  let slug = $state('');
  let transport = $state<MCPTransport>('http');
  let url = $state('');
  let image = $state('');
  let argvText = $state('');
  let enabled = $state(true);
  let host = $state('');
  let portText = $state('');
  let database = $state('');
  let user = $state('');
  let tlsMode = $state<MCPDBConfig['tlsMode']>('require');
  let readOnlyLogin = $state(false);
  let password = $state('');
  let passwordClear = $state(false);
  let passwordSaved = $state(false);
  let secrets = $state<SecretRow[]>([]);
  let tools = $state<MCPTool[]>([]);
  let discovered = $state<MCPDiscoveredTool[]>([]);
  let discoverNote = $state('');
  let discoverFailed = $state(false);
  let failure = $state(noFailure);
  let pullError = $state('');
  /** The request in flight: `save`, `discover`, `pull`, or `delete`. */
  let busy = $state('');
  let confirmDelete = $state(false);
  let form = $state<HTMLFormElement>();
  let autoRan = false;

  $effect(() => {
    const from = server ? null : entry;
    const kind = server?.transport ?? from?.transport ?? 'http';
    slug = server?.slug ?? (from ? suggestSlug(from.name) : '');
    transport = kind;
    url = server?.url ?? from?.url ?? '';
    image = server?.image ?? from?.image ?? '';
    argvText = (server?.argv ?? from?.argv ?? []).join('\n');
    enabled = server?.enabled ?? true;
    const config = server?.config ?? null;
    host = config?.host ?? '';
    portText = String(config?.port ?? from?.port ?? '');
    database = config?.database ?? '';
    user = config?.user ?? '';
    tlsMode = config?.tlsMode ?? 'require';
    readOnlyLogin = config?.readOnlyLogin ?? false;
    password = '';
    passwordClear = false;
    const db = isDatabase(kind);
    passwordSaved = db && (server?.secretsSet ?? []).includes('PASSWORD');
    secrets = server
      ? db
        ? []
        : server.secretsSet.map((name) => ({ name, value: '', clear: false, saved: true }))
      : (from?.secrets ?? [])
          .filter((secret) => !db || secret.name !== 'PASSWORD')
          .map((secret) => ({
            name: secret.name,
            value: '',
            clear: false,
            saved: false,
            hint: secret.description,
            required: secret.required
          }));
    tools = server
      ? server.tools.map((tool) => ({ ...tool }))
      : (from?.tools ?? []).map((tool) => ({ ...tool, schemaSha256: '' }));
  });

  $effect(() => {
    if (autoDiscover && server && !autoRan) {
      autoRan = true;
      void discover();
    }
  });

  const db = $derived(isDatabase(transport));
  const secretNoun = $derived(transport === 'http' ? 'header' : 'environment variable');
  const listed = $derived(new Map(tools.map((tool) => [tool.name, tool])));
  const canPull = $derived(transport === 'stdio' && image.trim() !== '' && !pinned.test(image.trim()));

  /** A `secrets` or `tools` error that names this row, so it shows on the row's control. */
  function rowError(fields: Record<string, string>, key: 'secrets' | 'tools', name: string): string {
    const text = fields[key] ?? '';
    const prefixes = key === 'secrets' ? [`secret name ${name} `] : [`tool ${name} `, `tool name ${name} `];
    return name !== '' && prefixes.some((prefix) => text.startsWith(prefix)) ? text : '';
  }

  /** The error keys a control on the form shows for this transport. */
  function shownFields(fields: Record<string, string>): string[] {
    const keys = ['slug', 'transport', 'enabled'];
    if (transport === 'http') keys.push('url');
    if (transport === 'stdio') keys.push('image', 'argv');
    if (db) {
      keys.push('config.host', 'config.port', 'config.database', 'config.user', 'config.tlsMode', 'config.readOnlyLogin');
      keys.push('secrets.PASSWORD', 'secrets');
    } else {
      for (const row of secrets) {
        const name = row.name.trim();
        if (name === '') continue;
        keys.push(`secrets.${name}`);
        if (rowError(fields, 'secrets', name)) keys.push('secrets');
      }
    }
    if (tools.some((tool) => rowError(fields, 'tools', tool.name))) keys.push('tools');
    return keys;
  }

  function addSecret() {
    secrets = [...secrets, { name: '', value: '', clear: false, saved: false }];
  }

  function body(): MCPServerInput {
    const values: Record<string, string> = {};
    const clear: string[] = [];
    if (db) {
      if (passwordClear) clear.push('PASSWORD');
      else if (password !== '') values.PASSWORD = password;
    } else {
      for (const row of secrets) {
        const name = row.name.trim();
        if (!name) continue;
        if (row.clear) clear.push(name);
        else if (row.value !== '') values[name] = row.value;
      }
    }
    return {
      slug: slug.trim(),
      transport,
      url: transport === 'http' ? url.trim() : '',
      image: transport === 'stdio' ? image.trim() : '',
      argv:
        transport === 'stdio'
          ? argvText
              .split('\n')
              .map((line) => line.trim())
              .filter((line) => line !== '')
          : [],
      config: db
        ? {
            host: host.trim(),
            port: Number(portText.trim() || defaultPort(transport)),
            database: database.trim(),
            user: user.trim(),
            tlsMode,
            readOnlyLogin
          }
        : null,
      enabled,
      secrets: values,
      clear,
      tools: tools.map((tool) => ({ name: tool.name, description: tool.description, schema: tool.schema }))
    };
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = 'save';
    failure = noFailure;
    pullError = '';
    try {
      if (server) {
        const saved = await putMCPServer(tribeId, server.id, body());
        server = saved;
        toast.success(`Saved ${saved.slug}`);
      } else {
        const saved = await createMCPServer(tribeId, body());
        toast.success(`Added ${saved.slug}`);
        await goto(`/tribes/${tribeId}/mcp/${saved.id}?discover=1`);
      }
    } catch (error) {
      failure = formFailure(error, shownFields(formFailure(error).fields));
      void focusInvalid(form);
    } finally {
      busy = '';
    }
  }

  async function discover() {
    if (!server) return;
    busy = 'discover';
    discoverNote = '';
    discoverFailed = false;
    try {
      const result = await discoverMCPTools(tribeId, server.id);
      discovered = result.tools;
      discoverFailed = !result.ok;
      discoverNote = result.ok
        ? `${result.tools.length} ${result.tools.length === 1 ? 'tool' : 'tools'} listed`
        : (result.message ?? 'The server could not be reached');
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      busy = '';
    }
  }

  async function pull() {
    busy = 'pull';
    pullError = '';
    try {
      const result = await pullMCPImage(tribeId, image.trim());
      image = result.image;
      toast.success('Image pinned');
    } catch (error) {
      const result = formFailure(error, ['image']);
      pullError = result.fields.image ?? '';
      if (!pullError) toast.error(failureText(error));
    } finally {
      busy = '';
    }
  }

  function allow(tool: MCPDiscoveredTool) {
    const next = { name: tool.name, description: tool.description, schema: tool.schema, schemaSha256: tool.schemaSha256 };
    tools = listed.has(tool.name) ? tools.map((t) => (t.name === tool.name ? next : t)) : [...tools, next];
  }

  async function remove() {
    if (!server) return;
    busy = 'delete';
    try {
      await deleteMCPServer(tribeId, server.id);
      toast.success(`Deleted ${server.slug}`);
      await goto(`/tribes/${tribeId}/mcp`);
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      busy = '';
    }
  }
</script>

<form onsubmit={save} bind:this={form} novalidate>
  <div class="flex flex-col gap-6">
    <FormAlert message={failure.message} extra={failure.extra} />
    {#if !server && entry}
      <Notice>
        Setting up <strong class="font-medium text-ink">{entry.title}</strong> from the catalog.
        {#if entry.note}{entry.note}{/if}
      </Notice>
    {/if}
    <Panel title="Server" hint="The slug names the tools: mcp_<slug>_<tool>.">
      <div class="grid gap-5 sm:grid-cols-2">
        <Field label="Slug" id="mcp-slug" width="full" mono error={failure.fields.slug} bind:value={slug} />
        <Select label="Transport" id="mcp-transport" width="full" error={failure.fields.transport} bind:value={transport}>
          <option value="http">Streamable HTTP</option>
          <option value="stdio">Stdio in the sandbox</option>
          <option value="postgres">PostgreSQL, built in</option>
          <option value="sqlserver">SQL Server, built in</option>
        </Select>
      </div>
      {#if transport === 'http'}
        <Field
          label="URL"
          id="mcp-url"
          width="full"
          mono
          hint="https only, with no query string. Private and loopback addresses are refused."
          error={failure.fields.url}
          bind:value={url}
        />
      {:else if transport === 'stdio'}
        <div class="flex flex-col gap-2">
          <Field
            label="Image"
            id="mcp-image"
            width="full"
            mono
            hint="Pinned by @sha256: digest. The server runs with no network."
            error={failure.fields.image || pullError}
            bind:value={image}
          />
          {#if canPull}
            <div>
              <Button type="button" variant="secondary" loading={busy === 'pull'} disabled={busy !== ''} onclick={() => void pull()}
                >Pull and pin image</Button
              >
            </div>
          {/if}
        </div>
        <Field
          label="Command"
          id="mcp-argv"
          width="full"
          mono
          type="textarea"
          rows={3}
          hint="One argument per line. The first is a program name, not a shell."
          error={failure.fields.argv}
          bind:value={argvText}
        />
      {/if}
      <Checkbox
        id="mcp-enabled"
        label="Enabled"
        hint="Jobs offer the tools of enabled servers only."
        error={failure.fields.enabled}
        bind:checked={enabled}
      />
    </Panel>

    {#if db}
      <Panel
        title="Connection"
        hint="The harness connects itself, to any host except the board database. Each call runs one read-only statement."
      >
        <div class="grid gap-5 sm:grid-cols-2">
          <Field label="Host" id="mcp-db-host" width="full" mono error={failure.fields['config.host']} bind:value={host} />
          <Field
            label="Port"
            id="mcp-db-port"
            width="full"
            mono
            placeholder={String(defaultPort(transport))}
            error={failure.fields['config.port']}
            bind:value={portText}
          />
          <Field label="Database" id="mcp-db-name" width="full" mono error={failure.fields['config.database']} bind:value={database} />
          <Field label="User" id="mcp-db-user" width="full" mono error={failure.fields['config.user']} bind:value={user} />
          <SecretField
            label="Password"
            name="mcp-db-password"
            saved={passwordSaved}
            width="full"
            error={failure.fields['secrets.PASSWORD'] || failure.fields.secrets}
            bind:value={password}
            bind:clear={passwordClear}
          />
          <Select label="TLS" id="mcp-db-tls" width="full" error={failure.fields['config.tlsMode']} bind:value={tlsMode}>
            <option value="require">Require, without checking the certificate</option>
            <option value="verify-full">Require and verify the certificate</option>
            <option value="disable">Disable</option>
          </Select>
        </div>
        <Checkbox
          id="mcp-db-readonly"
          label="This login can only read"
          hint={transport === 'sqlserver'
            ? 'Required. SQL Server has no read-only transaction, so use a login with db_datareader only.'
            : 'Required. Use a role with SELECT only; every call also runs in a read-only transaction.'}
          error={failure.fields['config.readOnlyLogin']}
          bind:checked={readOnlyLogin}
        />
      </Panel>
    {:else}
      <Panel
        title={transport === 'http' ? 'Headers' : 'Environment'}
        hint={`Each ${secretNoun} value is write-only and redacted from every event.`}
      >
        {#each secrets as row, index (index)}
          {#if row.saved}
            <SecretField
              label={row.name}
              name={`mcp-${row.name}`}
              saved
              width="lg"
              error={failure.fields[`secrets.${row.name}`]}
              bind:value={row.value}
              bind:clear={row.clear}
            />
          {:else}
            <div class="flex flex-col gap-1.5">
              <div class="grid gap-3 sm:grid-cols-2">
                <Field
                  label="Name"
                  id={`mcp-secret-name-${index}`}
                  width="full"
                  mono
                  error={rowError(failure.fields, 'secrets', row.name.trim())}
                  bind:value={row.name}
                />
                <SecretField
                  label="Value"
                  name={`mcp-new-${index}`}
                  width="full"
                  error={failure.fields[`secrets.${row.name.trim()}`]}
                  bind:value={row.value}
                  bind:clear={row.clear}
                />
              </div>
              {#if row.hint || row.required}
                <p class="text-xs text-muted">{row.required ? 'Required. ' : ''}{row.hint ?? ''}</p>
              {/if}
            </div>
          {/if}
        {/each}
        <div>
          <Button type="button" variant="secondary" icon="plus" disabled={busy !== ''} onclick={addSecret}>Add {secretNoun}</Button>
        </div>
      </Panel>
    {/if}

    <Panel title="Allowed tools" hint="Only read-only tools. The description is what the model reads.">
      {#if tools.length === 0}
        <EmptyState
          title="No tools allowed"
          description={server ? 'Discover the server’s tools, then allow the read-only ones.' : 'Save the server, then discover its tools.'}
        />
      {:else}
        {#each tools as tool (tool.name)}
          <div class="flex flex-col gap-2 border-b border-line pb-4 last:border-b-0">
            <div class="flex items-center justify-between gap-3">
              <span class="font-mono text-sm text-ink">{tool.name}</span>
              <Button
                type="button"
                variant="ghost"
                icon="trash"
                disabled={busy !== ''}
                onclick={() => (tools = tools.filter((t) => t.name !== tool.name))}>Remove</Button
              >
            </div>
            <Field
              label={`Description of ${tool.name}`}
              id={`mcp-tool-${tool.name}`}
              width="full"
              type="textarea"
              rows={2}
              error={rowError(failure.fields, 'tools', tool.name)}
              bind:value={tool.description}
            />
          </div>
        {/each}
      {/if}
      {#if server}
        <div class="flex flex-wrap items-center gap-3">
          <Button
            type="button"
            variant="secondary"
            icon="search"
            loading={busy === 'discover'}
            disabled={busy !== ''}
            onclick={() => void discover()}>Discover tools</Button
          >
          {#if discoverNote}<span class="text-sm {discoverFailed ? 'text-danger' : 'text-muted'}">{discoverNote}</span>{/if}
        </div>
      {/if}
      {#if discovered.length > 0}
        <Table label="Discovered tools">
          <thead>
            <tr class="border-b border-line bg-ground/50">
              <th class={th} scope="col">Tool</th>
              <th class={th} scope="col">Server description</th>
              <th class={th} scope="col"><span class="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            {#each discovered as tool (tool.name)}
              {@const current = listed.get(tool.name)}
              <tr class={tr}>
                <td class="{td} font-mono">{tool.name}</td>
                <td class="{td} text-muted">{tool.description}</td>
                <td class="{td} text-right">
                  {#if !tool.valid}
                    <Badge tone="red">Name not allowed</Badge>
                  {:else if current && current.schemaSha256 === tool.schemaSha256}
                    <Badge tone="green">Allowed</Badge>
                  {:else if current}
                    <Button type="button" variant="ghost" disabled={busy !== ''} onclick={() => allow(tool)}>Update schema</Button>
                  {:else}
                    <Button type="button" variant="ghost" disabled={busy !== ''} onclick={() => allow(tool)}>Allow</Button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </Table>
      {/if}
      {#snippet footer()}
        <Button
          variant="primary"
          type="submit"
          icon={server ? 'save' : 'plus'}
          loading={busy === 'save'}
          disabled={busy !== ''}>{server ? 'Save' : 'Add server'}</Button
        >
      {/snippet}
    </Panel>
  </div>
</form>

{#if server}
  <div class="mt-6">
    <Panel title="Delete server" hint="Jobs that start after this stop offering its tools.">
      {#if confirmDelete}
        <Notice tone="warning">Delete {server.slug}? Its allowlist and secrets are removed.</Notice>
      {/if}
      {#snippet footer()}
        {#if confirmDelete}
          <Button type="button" variant="ghost" disabled={busy !== ''} onclick={() => (confirmDelete = false)}>Cancel</Button>
          <Button type="button" variant="danger" icon="trash" loading={busy === 'delete'} disabled={busy !== ''} onclick={() => void remove()}
            >Delete</Button
          >
        {:else}
          <Button type="button" variant="danger" icon="trash" disabled={busy !== ''} onclick={() => (confirmDelete = true)}>Delete server</Button>
        {/if}
      {/snippet}
    </Panel>
  </div>
{/if}
