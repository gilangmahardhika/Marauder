import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { toast } from '$lib/toast.svelte';
import type { MCPCatalogEntry, MCPServer } from '$lib/types';

const api = vi.hoisted(() => ({
  getMCPServers: vi.fn(),
  getTribes: vi.fn(),
  createMCPServer: vi.fn(),
  putMCPServer: vi.fn(),
  deleteMCPServer: vi.fn(),
  discoverMCPTools: vi.fn(),
  getMCPCatalog: vi.fn(),
  getMCPCatalogEntry: vi.fn(),
  pullMCPImage: vi.fn()
}));
const nav = vi.hoisted(() => ({
  goto: vi.fn(),
  params: { id: 'tribe_gadget', serverId: 'mcp_1' } as Record<string, string>,
  href: 'http://127.0.0.1:4173/tribes/tribe_gadget/mcp'
}));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({ params: nav.params, url: new URL(nav.href) }).subscribe(run)
  }
}));
vi.mock('$app/navigation', () => ({ goto: nav.goto }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {
    fields: Record<string, string> = {};
  },
  unreachable: () => 'API unreachable',
  ...api
}));

import ListPage from './+page.svelte';
import EditPage from './[serverId]/+page.svelte';
import CatalogPage from './catalog/+page.svelte';
import NewPage from './new/+page.svelte';

const server: MCPServer = {
  id: 'mcp_1',
  tribeId: 'tribe_gadget',
  slug: 'tickets',
  transport: 'http',
  url: 'https://mcp.example.com/v1',
  image: '',
  argv: [],
  config: null,
  enabled: true,
  secretsSet: ['Authorization'],
  tools: [],
  updatedAt: '2026-10-13T00:00:00Z'
};

beforeEach(() => {
  api.getTribes.mockResolvedValue({ tribes: [{ id: 'tribe_gadget', name: 'Gadget tribe' }] });
  api.getMCPServers.mockResolvedValue({ servers: [server] });
  api.putMCPServer.mockImplementation((_tribe: string, _id: string, body: { tools: MCPServer['tools'] }) =>
    Promise.resolve({ ...server, tools: body.tools })
  );
  api.createMCPServer.mockResolvedValue({ ...server, id: 'mcp_2', slug: 'docs' });
});

afterEach(() => {
  vi.clearAllMocks();
  toast.clear();
  nav.href = 'http://127.0.0.1:4173/tribes/tribe_gadget/mcp';
});

test('lists the tribe servers with links to add and edit', async () => {
  render(ListPage);
  expect(await screen.findByRole('heading', { name: 'MCP servers: Gadget tribe' })).toBeTruthy();
  expect(screen.getByText('tickets')).toBeTruthy();
  expect(screen.getByText('https://mcp.example.com/v1')).toBeTruthy();
  expect(screen.getByRole('link', { name: 'Edit' }).getAttribute('href')).toBe('/tribes/tribe_gadget/mcp/mcp_1');
  expect(screen.getByRole('link', { name: 'Add manually' }).getAttribute('href')).toBe('/tribes/tribe_gadget/mcp/new');
  expect(screen.getByRole('link', { name: 'Browse catalog' }).getAttribute('href')).toBe('/tribes/tribe_gadget/mcp/catalog');
});

test('keeps a saved header, and allows a discovered tool', async () => {
  api.discoverMCPTools.mockResolvedValue({
    ok: true,
    tools: [
      { name: 'lookup', description: 'Find a ticket', schema: { type: 'object' }, schemaSha256: 'abc', valid: true },
      { name: 'bad.name', description: '', schema: {}, schemaSha256: 'def', valid: false }
    ]
  });
  render(EditPage);
  expect(await screen.findByRole('heading', { name: 'MCP server: tickets' })).toBeTruthy();
  const secret = screen.getByLabelText('Authorization') as HTMLInputElement;
  expect(secret.value).toBe('');
  expect(secret.placeholder).toBe('A value is saved');

  await fireEvent.click(screen.getByRole('button', { name: 'Discover tools' }));
  expect(await screen.findByText('Name not allowed')).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: 'Allow' }));
  expect(screen.getByLabelText('Description of lookup')).toBeTruthy();

  await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(api.putMCPServer).toHaveBeenCalled());
  const [tribe, id, body] = api.putMCPServer.mock.calls[0] as [string, string, Record<string, unknown>];
  expect(tribe).toBe('tribe_gadget');
  expect(id).toBe('mcp_1');
  expect(body.secrets).toEqual({});
  expect(body.clear).toEqual([]);
  expect(body.tools).toEqual([{ name: 'lookup', description: 'Find a ticket', schema: { type: 'object' } }]);
  await waitFor(() => expect(toast.items.map((item) => item.text)).toEqual(['Saved tickets']));
});

test('shows a failed discovery as its fixed message', async () => {
  api.discoverMCPTools.mockResolvedValue({ ok: false, message: 'the server address was refused', tools: [] });
  render(EditPage);
  await fireEvent.click(await screen.findByRole('button', { name: 'Discover tools' }));
  expect(await screen.findByText('the server address was refused')).toBeTruthy();
});

test('creates a stdio server with an environment secret', async () => {
  render(NewPage);
  await fireEvent.input(screen.getByLabelText('Slug'), { target: { value: 'docs' } });
  const transport = screen.getByLabelText('Transport') as HTMLSelectElement;
  transport.value = 'stdio';
  await fireEvent.change(transport);
  await fireEvent.input(screen.getByLabelText('Image'), { target: { value: 'ghcr.io/acme/docs@sha256:' + 'a'.repeat(64) } });
  await fireEvent.input(screen.getByLabelText('Command'), { target: { value: 'mcp-docs\n--read-only\n' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Add environment variable' }));
  await fireEvent.input(screen.getByLabelText('Name'), { target: { value: 'DOCS_TOKEN' } });
  await fireEvent.input(screen.getByLabelText('Value'), { target: { value: 'docs-token-123' } });
  expect(screen.queryByRole('button', { name: 'Discover tools' })).toBeNull();
  await fireEvent.click(screen.getByRole('button', { name: 'Add server' }));
  await waitFor(() => expect(api.createMCPServer).toHaveBeenCalled());
  const [tribe, body] = api.createMCPServer.mock.calls[0] as [string, Record<string, unknown>];
  expect(tribe).toBe('tribe_gadget');
  expect(body).toMatchObject({
    slug: 'docs',
    transport: 'stdio',
    url: '',
    argv: ['mcp-docs', '--read-only'],
    config: null,
    secrets: { DOCS_TOKEN: 'docs-token-123' },
    tools: []
  });
  await waitFor(() => expect(nav.goto).toHaveBeenCalledWith('/tribes/tribe_gadget/mcp/mcp_2?discover=1'));
  expect(toast.items.map((item) => item.text)).toEqual(['Added docs']);
});

test('a refused secret name shows on its row and a connection error in the alert', async () => {
  const { ApiError } = await import('$lib/api');
  const refused = new ApiError(400, 'validation', 'validation');
  refused.fields = {
    secrets: 'secret name bad name is not a valid header, environment, or PASSWORD name',
    tools: 'a server has at most 32 tools'
  };
  api.createMCPServer.mockRejectedValueOnce(refused);
  render(NewPage);
  await fireEvent.input(screen.getByLabelText('Slug'), { target: { value: 'docs' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Add header' }));
  await fireEvent.input(screen.getByLabelText('Name'), { target: { value: 'bad name' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Add server' }));
  expect(await screen.findByText(/secret name bad name is not a valid header/)).toBeTruthy();
  expect(screen.getByLabelText('Name').getAttribute('aria-invalid')).toBe('true');
  const alert = screen.getByRole('alert');
  expect(alert.textContent).toContain('Check the fields marked below.');
  expect(alert.textContent).toContain('a server has at most 32 tools');
  expect(alert.textContent).not.toContain('secret name bad name');
});

const dbTools = [
  { name: 'describe_table', description: "List a table's columns.", schema: { type: 'object' } },
  { name: 'list_tables', description: 'List the tables.', schema: { type: 'object' } },
  { name: 'query', description: 'Run one read-only statement.', schema: { type: 'object' } }
];

const postgresEntry: MCPCatalogEntry = {
  id: 'builtin:postgres',
  source: 'builtin',
  name: 'postgres',
  title: 'PostgreSQL',
  description: 'Read-only SQL on a Postgres database.',
  websiteUrl: '',
  version: '',
  transport: 'postgres',
  supported: true,
  argv: [],
  port: 5432,
  secrets: [{ name: 'PASSWORD', description: 'Password of a login that can only read.', required: false }],
  tools: dbTools
};

const ticketsEntry: MCPCatalogEntry = {
  id: 'registry:io.example/tickets@2.0.0',
  source: 'registry',
  name: 'io.example/tickets',
  title: 'Tickets',
  description: 'Find tickets',
  websiteUrl: 'https://tickets.example.com',
  version: '2.0.0',
  transport: 'http',
  supported: true,
  url: 'https://mcp.tickets.example.com/mcp',
  argv: [],
  secrets: [{ name: 'Authorization', description: 'Bearer token', required: true }],
  tools: []
};

const docsEntry: MCPCatalogEntry = {
  id: 'registry:io.github.acme/docs@0.3.1',
  source: 'registry',
  name: 'io.github.acme/docs',
  title: 'docs',
  description: 'Docs search',
  websiteUrl: '',
  version: '0.3.1',
  transport: 'stdio',
  supported: true,
  note: 'Runs with no network. Pull and pin the image before saving.',
  image: 'ghcr.io/acme/docs-mcp:0.3.1',
  argv: ['stdio'],
  secrets: [{ name: 'DOCS_TOKEN', description: 'API token', required: true }],
  tools: []
};

const npmEntry: MCPCatalogEntry = {
  id: 'registry:io.github.acme/pg-npm@1.0.0',
  source: 'registry',
  name: 'io.github.acme/pg-npm',
  title: 'pg-npm',
  description: '',
  websiteUrl: '',
  version: '1.0.0',
  transport: 'stdio',
  supported: false,
  reason: 'npm packages need a package runner, which the sandbox does not have.',
  argv: [],
  secrets: [],
  tools: []
};

const githubEntry: MCPCatalogEntry = {
  id: 'featured:github',
  source: 'featured',
  name: 'github',
  title: 'GitHub',
  description: "Read repositories, issues, pull requests, and code search through GitHub's read-only endpoint.",
  websiteUrl: 'https://github.com/github/github-mcp-server',
  version: '',
  transport: 'http',
  supported: true,
  url: 'https://api.githubcopilot.com/mcp/readonly',
  argv: [],
  secrets: [{ name: 'Authorization', description: 'Bearer, then a fine-grained personal access token with read access.', required: true }],
  tools: []
};

const deepwikiEntry: MCPCatalogEntry = {
  ...githubEntry,
  id: 'featured:deepwiki',
  name: 'deepwiki',
  title: 'DeepWiki',
  description: 'Ask questions about public GitHub repositories.',
  websiteUrl: 'https://deepwiki.com',
  url: 'https://mcp.deepwiki.com/mcp',
  secrets: []
};

type Pending = { resolve: (page: { entries: MCPCatalogEntry[]; nextCursor: string; registryError?: string }) => void; signal?: AbortSignal };

test('catalog shows featured and built-in servers before the registry answers', async () => {
  const registryCalls: Pending[] = [];
  api.getMCPCatalog.mockImplementation((_tribe: string, source: string, q: string, _cursor: string, signal?: AbortSignal) => {
    if (source === 'local') {
      const entries = [postgresEntry, githubEntry, deepwikiEntry].filter((e) => !q || e.title.toLowerCase().includes(q));
      return Promise.resolve({ entries, nextCursor: '' });
    }
    return new Promise((resolve) => registryCalls.push({ resolve, signal }));
  });
  render(CatalogPage);
  expect(await screen.findByRole('heading', { name: 'GitHub' })).toBeTruthy();
  expect(api.getMCPCatalog).toHaveBeenCalledWith('tribe_gadget', 'local', '', '', expect.any(AbortSignal));
  expect(api.getMCPCatalog).toHaveBeenCalledWith('tribe_gadget', 'registry', '', '', expect.any(AbortSignal));
  expect(screen.getByRole('list', { name: 'Featured servers' })).toBeTruthy();
  expect(screen.getByRole('list', { name: 'Database connectors' })).toBeTruthy();
  expect(screen.getByText('Token needed')).toBeTruthy();
  expect(screen.getByText('No sign-in')).toBeTruthy();
  expect(screen.getByText('Built-in')).toBeTruthy();
  expect(screen.getByText('Searching the MCP Registry…')).toBeTruthy();
  expect(screen.getByRole('link', { name: 'Set up GitHub' }).getAttribute('href')).toBe('/tribes/tribe_gadget/mcp/new?from=featured%3Agithub');
  expect(screen.getByRole('link', { name: 'Set up PostgreSQL' }).getAttribute('href')).toBe('/tribes/tribe_gadget/mcp/new?from=builtin%3Apostgres');

  registryCalls[0].resolve({ entries: [ticketsEntry, docsEntry, npmEntry], nextCursor: 'next-1' });
  expect(await screen.findByRole('heading', { name: 'Tickets' })).toBeTruthy();
  expect(screen.getByText('3 servers')).toBeTruthy();
  expect(screen.getByText('No network')).toBeTruthy();
  expect(screen.getByText(/Cannot be set up: npm packages need a package runner/)).toBeTruthy();
  const registryLinks = screen.getAllByRole('link', { name: /^Set up / }).map((link) => link.getAttribute('href'));
  expect(registryLinks).toEqual([
    '/tribes/tribe_gadget/mcp/new?from=featured%3Agithub',
    '/tribes/tribe_gadget/mcp/new?from=featured%3Adeepwiki',
    '/tribes/tribe_gadget/mcp/new?from=builtin%3Apostgres',
    '/tribes/tribe_gadget/mcp/new?from=registry%3Aio.example%2Ftickets%402.0.0',
    '/tribes/tribe_gadget/mcp/new?from=registry%3Aio.github.acme%2Fdocs%400.3.1'
  ]);

  await fireEvent.click(screen.getByRole('button', { name: 'Load more' }));
  await waitFor(() => expect(api.getMCPCatalog).toHaveBeenCalledWith('tribe_gadget', 'registry', '', 'next-1', expect.any(AbortSignal)));
  registryCalls[1].resolve({ entries: [], nextCursor: '' });

  // Typing aborts the slow search, shows local matches at once, and keeps waiting only on the registry.
  await fireEvent.input(screen.getByLabelText('Search'), { target: { value: 'deep' } });
  await waitFor(() => expect(api.getMCPCatalog).toHaveBeenCalledWith('tribe_gadget', 'local', 'deep', '', expect.any(AbortSignal)));
  await waitFor(() => expect(screen.queryByRole('heading', { name: 'GitHub' })).toBeNull());
  expect(screen.getByRole('heading', { name: 'DeepWiki' })).toBeTruthy();
  await fireEvent.input(screen.getByLabelText('Search'), { target: { value: 'deepw' } });
  await waitFor(() => expect(api.getMCPCatalog).toHaveBeenCalledWith('tribe_gadget', 'registry', 'deepw', '', expect.any(AbortSignal)));
  const stale = registryCalls.find((c) => api.getMCPCatalog.mock.calls.some((call: unknown[]) => call[2] === 'deep' && call[4] === c.signal));
  expect(stale?.signal?.aborted).toBe(true);
  registryCalls[registryCalls.length - 1].resolve({ entries: [], nextCursor: '', registryError: 'The MCP Registry could not be reached' });
  expect(await screen.findByText(/The MCP Registry could not be reached. The featured and built-in servers are still listed/)).toBeTruthy();
  expect(screen.getByRole('heading', { name: 'DeepWiki' })).toBeTruthy();
});

test('sets up Postgres from the catalog with its fixed tools and a read-only login', async () => {
  nav.href = 'http://127.0.0.1:4173/tribes/tribe_gadget/mcp/new?from=builtin%3Apostgres';
  api.getMCPCatalogEntry.mockResolvedValue(postgresEntry);
  const { ApiError } = await import('$lib/api');
  const refused = new ApiError(400, 'validation', 'validation');
  refused.fields = { 'config.readOnlyLogin': 'confirm that this login can only read' };
  api.createMCPServer.mockRejectedValueOnce(refused);
  render(NewPage);
  expect(await screen.findByRole('heading', { name: 'Set up PostgreSQL' })).toBeTruthy();
  expect(api.getMCPCatalogEntry).toHaveBeenCalledWith('tribe_gadget', 'builtin:postgres');
  expect((screen.getByLabelText('Slug') as HTMLInputElement).value).toBe('postgres');
  expect((screen.getByLabelText('Transport') as HTMLSelectElement).value).toBe('postgres');
  expect((screen.getByLabelText('Port') as HTMLInputElement).value).toBe('5432');
  expect(screen.getByLabelText('Description of query')).toBeTruthy();
  expect(screen.queryByLabelText('URL')).toBeNull();

  await fireEvent.input(screen.getByLabelText('Host'), { target: { value: '10.0.0.5' } });
  await fireEvent.input(screen.getByLabelText('Database'), { target: { value: 'app' } });
  await fireEvent.input(screen.getByLabelText('User'), { target: { value: 'reader' } });
  await fireEvent.input(screen.getByLabelText('Password'), { target: { value: 'reader-password' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Add server' }));
  expect(await screen.findByText('confirm that this login can only read')).toBeTruthy();
  expect(screen.getByLabelText(/This login can only read/).getAttribute('aria-invalid')).toBe('true');
  expect(screen.getByRole('alert').textContent).toContain('Check the fields marked below.');
  expect((screen.getByLabelText('Password') as HTMLInputElement).value).toBe('reader-password');

  await fireEvent.click(screen.getByLabelText(/This login can only read/));
  await fireEvent.click(screen.getByRole('button', { name: 'Add server' }));
  await waitFor(() => expect(api.createMCPServer).toHaveBeenCalledTimes(2));
  const [, body] = api.createMCPServer.mock.calls[1] as [string, Record<string, unknown>];
  expect(body).toMatchObject({
    slug: 'postgres',
    transport: 'postgres',
    url: '',
    image: '',
    argv: [],
    config: { host: '10.0.0.5', port: 5432, database: 'app', user: 'reader', tlsMode: 'require', readOnlyLogin: true },
    secrets: { PASSWORD: 'reader-password' },
    clear: []
  });
  expect((body.tools as { name: string }[]).map((tool) => tool.name)).toEqual(['describe_table', 'list_tables', 'query']);
});

test('fills a registry server URL and header names from the catalog', async () => {
  nav.href = 'http://127.0.0.1:4173/tribes/tribe_gadget/mcp/new?from=registry%3Aio.example%2Ftickets%402.0.0';
  api.getMCPCatalogEntry.mockResolvedValue(ticketsEntry);
  render(NewPage);
  expect(await screen.findByRole('heading', { name: 'Set up Tickets' })).toBeTruthy();
  expect((screen.getByLabelText('Slug') as HTMLInputElement).value).toBe('tickets');
  expect((screen.getByLabelText('URL') as HTMLInputElement).value).toBe('https://mcp.tickets.example.com/mcp');
  expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('Authorization');
  expect(screen.getByText('Required. Bearer token')).toBeTruthy();
  await fireEvent.input(screen.getByLabelText('Value'), { target: { value: 'Bearer ticket-token-1' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Add server' }));
  await waitFor(() => expect(api.createMCPServer).toHaveBeenCalled());
  const [, body] = api.createMCPServer.mock.calls[0] as [string, Record<string, unknown>];
  expect(body).toMatchObject({ transport: 'http', config: null, secrets: { Authorization: 'Bearer ticket-token-1' } });
});

test('pulls and pins a stdio image from the catalog', async () => {
  nav.href = 'http://127.0.0.1:4173/tribes/tribe_gadget/mcp/new?from=registry%3Aio.github.acme%2Fdocs%400.3.1';
  const pinned = 'ghcr.io/acme/docs-mcp@sha256:' + 'b'.repeat(64);
  api.getMCPCatalogEntry.mockResolvedValue(docsEntry);
  api.pullMCPImage.mockResolvedValue({ image: pinned });
  render(NewPage);
  expect(await screen.findByText(/Runs with no network/)).toBeTruthy();
  expect((screen.getByLabelText('Command') as HTMLTextAreaElement).value).toBe('stdio');
  await fireEvent.click(screen.getByRole('button', { name: 'Pull and pin image' }));
  await waitFor(() => expect((screen.getByLabelText('Image') as HTMLInputElement).value).toBe(pinned));
  expect(api.pullMCPImage).toHaveBeenCalledWith('tribe_gadget', 'ghcr.io/acme/docs-mcp:0.3.1');
  expect(screen.queryByRole('button', { name: 'Pull and pin image' })).toBeNull();
});

test('discovers tools as soon as a new server opens', async () => {
  nav.href = 'http://127.0.0.1:4173/tribes/tribe_gadget/mcp/mcp_1?discover=1';
  api.discoverMCPTools.mockResolvedValue({ ok: true, tools: [] });
  render(EditPage);
  await waitFor(() => expect(api.discoverMCPTools).toHaveBeenCalledWith('tribe_gadget', 'mcp_1'));
  expect(api.discoverMCPTools).toHaveBeenCalledTimes(1);
});

test('lists a database server by host and database', async () => {
  api.getMCPServers.mockResolvedValue({
    servers: [
      {
        ...server,
        slug: 'pg',
        transport: 'postgres',
        url: '',
        config: { host: '10.0.0.5', port: 5432, database: 'app', user: 'reader', tlsMode: 'require', readOnlyLogin: true }
      }
    ]
  });
  render(ListPage);
  expect(await screen.findByText('postgres · 10.0.0.5:5432/app')).toBeTruthy();
});
