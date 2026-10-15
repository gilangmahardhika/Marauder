import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { shell } from '$lib/shell.svelte';
import { toast } from '$lib/toast.svelte';
import type { Repository } from '$lib/types';

const api = vi.hoisted(() => ({
  getCompanyRepos: vi.fn(),
  getRepository: vi.fn(),
  putRepository: vi.fn(),
  syncRepository: vi.fn()
}));

vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
  unreachable: () => 'API unreachable',
  ...api
}));

import RepositoryPage from './+page.svelte';

const view: Repository = {
  host: 'github.com',
  org: 'acme',
  repo: 'acme/shop',
  baseBranch: 'main',
  cloned: true,
  headSha: 'abc123',
  prdPath: 'docs/prd',
  rfcPath: 'docs/rfc',
  canEdit: true
};

beforeEach(() => {
  shell.projectId = 'proj_a';
  api.getRepository.mockResolvedValue(view);
  api.getCompanyRepos.mockResolvedValue({ repos: [{ name: 'acme/shop', defaultBranch: 'main', archived: false }] });
});

afterEach(() => {
  vi.clearAllMocks();
  toast.clear();
  shell.projectId = '';
});

test('a sync without the company token says why in an error toast', async () => {
  api.syncRepository.mockRejectedValue(
    Object.assign(new Error('no company token'), { fields: { token: 'an admin must save the company GitHub token' } })
  );
  render(RepositoryPage);
  await fireEvent.click(await screen.findByRole('button', { name: 'Sync' }));
  await waitFor(() =>
    expect(toast.items.map((item) => [item.tone, item.text])).toEqual([['error', 'an admin must save the company GitHub token']])
  );
});

test('the Docs panel loads the doc paths and saves them with the repository', async () => {
  api.putRepository.mockImplementation((body: Record<string, unknown>) => Promise.resolve({ ...view, ...body }));
  render(RepositoryPage);
  const prd = (await screen.findByLabelText('PRD path')) as HTMLInputElement;
  await waitFor(() => expect(prd.value).toBe('docs/prd'));
  expect((screen.getByLabelText('RFC path') as HTMLInputElement).value).toBe('docs/rfc');

  await fireEvent.input(prd, { target: { value: 'specs/prd' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(api.putRepository).toHaveBeenCalledTimes(1));
  expect(api.putRepository.mock.calls[0][0]).toEqual({
    repo: 'acme/shop',
    baseBranch: 'main',
    prdPath: 'specs/prd',
    rfcPath: 'docs/rfc'
  });
});

test('a bad doc path shows under its field', async () => {
  api.putRepository.mockRejectedValue(
    Object.assign(new Error('validation failed'), { fields: { prdPath: "path is relative to the repository, with no '..'" } })
  );
  render(RepositoryPage);
  const prd = await screen.findByLabelText('PRD path');
  await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  expect(await screen.findByText("path is relative to the repository, with no '..'")).toBeTruthy();
  expect(prd.getAttribute('aria-invalid')).toBe('true');
});

test('a save error with no input shows in the form alert', async () => {
  api.putRepository.mockRejectedValue(
    Object.assign(new Error('validation failed'), {
      fields: { org: 'an admin must set the company organization', baseBranch: 'branch not found' }
    })
  );
  render(RepositoryPage);
  await fireEvent.click(await screen.findByRole('button', { name: 'Save' }));
  expect(await screen.findByText('branch not found')).toBeTruthy();
  expect(screen.getByLabelText('Base branch').getAttribute('aria-invalid')).toBe('true');
  expect(screen.getByRole('alert').textContent).toContain('an admin must set the company organization');
});
