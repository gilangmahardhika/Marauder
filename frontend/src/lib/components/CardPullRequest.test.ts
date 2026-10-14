import { fireEvent, render, screen } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import CardPullRequest from './CardPullRequest.svelte';
import type { CardPullRequest as PullRequest } from '$lib/types';

const pullRequest: PullRequest = {
  title: 'REQ-001: Change login label to Username only',
  body: '## Summary\n\nThe login form asked for a **username or an email**.\n\n## Verification\n\nLocal check `make test` exited 0.',
  attemptN: 2,
  sha: 'ae5d4418f1f5c54e02c2ced82de2a5b66ba65753'
};
const prUrl = 'https://github.com/acme/widget/pull/7';

test('shows the title, branch, commit, and rendered description', () => {
  render(CardPullRequest, { pullRequest, prUrl, branch: 'orch/card_1', base: 'main' });
  expect(screen.getByRole('heading', { name: 'Pull request #7' })).toBeTruthy();
  expect(screen.getByRole('heading', { name: pullRequest.title })).toBeTruthy();
  expect(screen.getByText('orch/card_1')).toBeTruthy();
  expect(screen.getByText('main')).toBeTruthy();
  expect(screen.getByText('ae5d441').getAttribute('title')).toBe(pullRequest.sha);
  expect(screen.getByText('Attempt 2')).toBeTruthy();
  expect(screen.getByRole('heading', { name: 'Summary' })).toBeTruthy();
  expect(screen.getByText('username or an email').tagName).toBe('STRONG');
  expect(screen.getByRole('link', { name: /Open on GitHub/ }).getAttribute('href')).toBe(prUrl);
  expect(screen.queryByRole('button', { name: /Show full description/ })).toBeNull();
});

test('a long description collapses behind a toggle', async () => {
  render(CardPullRequest, {
    pullRequest: { ...pullRequest, body: `## Summary\n\n${'word '.repeat(400)}` },
    prUrl,
    branch: 'orch/card_1'
  });
  const toggle = screen.getByRole('button', { name: /Show full description/ });
  expect(toggle.getAttribute('aria-expanded')).toBe('false');
  expect(document.getElementById('pr-description')!.className).toContain('max-h-80');
  await fireEvent.click(toggle);
  expect(toggle.getAttribute('aria-expanded')).toBe('true');
  expect(document.getElementById('pr-description')!.className).not.toContain('max-h-80');
});

test('explains a card with no stored text', () => {
  const { unmount } = render(CardPullRequest, { pullRequest: null, prUrl: null, branch: null });
  expect(screen.getByText(/No pull request yet/)).toBeTruthy();
  unmount();
  render(CardPullRequest, { pullRequest: null, prUrl, branch: 'orch/card_1' });
  expect(screen.getByText(/published before the orchestrator stored pull request text/)).toBeTruthy();
  expect(screen.getByRole('link', { name: /Open on GitHub/ })).toBeTruthy();
});

test('does not link a pull request outside GitHub', () => {
  render(CardPullRequest, { pullRequest, prUrl: 'https://evil.example/pull/7', branch: null });
  expect(screen.queryByRole('link', { name: /Open on GitHub/ })).toBeNull();
});
