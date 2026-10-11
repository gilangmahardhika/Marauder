const statuses: Record<string, string> = {
  backlog: 'Backlog',
  ready: 'Ready',
  in_progress: 'In progress',
  self_check: 'Running checks',
  pr_open: 'Pull request open',
  agent_review: 'AI review',
  changes_requested: 'Changes requested',
  human_review: 'Waiting for review',
  approved: 'Approved',
  deploying_dev: 'Deploying to dev',
  verifying: 'Verifying on dev',
  done: 'Done',
  blocked: 'Blocked'
};

const reasons: Record<string, string> = {
  owner_github: "Owner's GitHub not verified",
  source_changed: 'Requirement changed',
  attempt_cap: 'Out of attempts',
  rejected: 'Rejected',
  deploy_failed: 'Deploy failed',
  deploy_config: 'Deploy not set up',
  github_config: 'GitHub not set up',
  no_commit: 'No commit',
  assertion_failed: 'Dev check failed',
  ci: 'CI failed'
};

/** "some_code" reads as "Some code". Text that is not a code is returned as is. */
function sentence(value: string) {
  if (!/^[a-z0-9_]+$/.test(value)) return value;
  const words = value.replaceAll('_', ' ').trim();
  return words.charAt(0).toUpperCase() + words.slice(1);
}

export function statusLabel(status: string) {
  return statuses[status] ?? sentence(status);
}

export function reasonLabel(reason: string) {
  return reasons[reason] ?? sentence(reason);
}
