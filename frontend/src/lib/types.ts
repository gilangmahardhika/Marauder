export type CardOwner = {
  id: string;
  name: string;
  email: string;
  githubLogin: string;
};

export type Card = {
  id: string;
  title: string;
  status: string;
  group: string;
  actions: string[];
  unblockTo: string | null;
  owner: CardOwner | null;
  reviewers: Reviewer[];
  skills: SkillRef[];
  requirements?: CardRequirement[];
  rules?: CardRule[];
  notes?: CardNote[];
  /** Single card only. What every later attempt reads with the newest note (RFC-073). */
  instructions?: string | null;
  requirementIds: string[];
  decisionIds: string[];
  dependsOn: string[];
  /** True while an open run request waits for the worker. */
  queued: boolean;
  /** Dependencies that are not done yet. */
  waitingOn: string[];
  risk: string;
  service: string;
  manual: boolean;
  localCheck: string | null;
  devCheck: string | null;
  attemptsUsed: number;
  maxAttempts: number;
  branch: string | null;
  prUrl: string | null;
  headSha: string | null;
  reviewedSha: string | null;
  reviewIndependent: boolean | null;
  /** `failed` means the AI review gave no verdict and the pull request checks decided. */
  reviewVerdict: 'recommend_approve' | 'request_changes' | 'failed' | null;
  /** Every requirement of the card has Self review on, so its owner may approve. */
  selfReview: boolean;
  /** `manual` when any requirement of the card is manual (RFC-072). */
  aiReviewMode: AIReviewMode;
  approvedSha: string | null;
  deployedSha: string | null;
  environment: string;
  failureReason: string | null;
  blockedReason: string | null;
  updatedAt: string;
};

export type SkillRef = {
  id: string;
  slug: string;
  title: string;
  useWhen: string;
};

/** On the single-card response only. */
export type CardRequirement = {
  id: string;
  heading: string;
  statement: string;
  sourceKind: string;
  sourcePath: string;
  accepted: boolean;
};

/** On the single-card response only. `label` is "global" or the stack name. */
export type CardRule = {
  id: string;
  title: string;
  label: string;
  category: string;
};

/** On the single-card response only, newest first. `attemptN` is 0 when no attempt wrote the note. */
export type CardNote = {
  source: string;
  note: string;
  createdAt: string;
  attemptN: number;
};

export type Board = {
  canRun: boolean;
  /** Ready cards whose dependencies are done and that are not queued. */
  readyCount: number;
  running: boolean;
  cards: Card[];
};

export type TraceEvent = {
  id: string;
  ts: string;
  attemptId?: string;
  kind: string;
  payload: unknown;
};

/** Empty strings are values the attempt does not have yet. */
export type TraceAttempt = {
  id: string;
  n: number;
  taskClass: string;
  modelId: string;
  fallbackModelId: string;
  inputTokens: number;
  outputTokens: number;
  commitSha: string;
  reviewModelId: string;
  reviewVerdict: string;
  reviewIndependent: boolean | null;
  startedAt: string;
  endedAt: string;
  outcome: string;
};

export type TraceResponse = { attempts: TraceAttempt[]; events: TraceEvent[] };

export type Requirement = {
  id: string;
  sourceKind: 'prd' | 'rfc' | 'draft' | 'manual';
  sourcePath: string;
  heading: string;
  statement: string;
  acceptance: string[];
  accepted: boolean;
  createdBy: string | null;
  linkedCards: string[];
  reviewers: Reviewer[];
  canEditReviewers: boolean;
  /** The owner of this requirement's cards may approve them alone (RFC-070). */
  selfReview: boolean;
  canSetSelfReview: boolean;
  /** What an AI request for changes does on this requirement's cards (RFC-072). */
  aiReviewMode: AIReviewMode;
};

export type AIReviewMode = 'continue' | 'manual';

export type Reviewer = {
  id: string;
  name: string;
  email: string;
  githubLogin: string;
};

export type RequirementDraft = {
  statement: string;
  acceptance: string[];
};

export type Decision = {
  id: string;
  sourcePath: string;
  statement: string;
};

export type Intake = {
  draftPending: boolean;
  planError: string | null;
  canAccept: boolean;
  skipped?: string[];
  requirements: Requirement[];
  decisions: Decision[];
  reviewerOptions: Reviewer[];
};

export type SessionUser = {
  id: string;
  name: string;
  email: string;
  role: string;
  admin: boolean;
  disabled?: boolean;
};

export type TribeMember = SessionUser & {
  tribeAdmin: boolean;
};

export type TribeCandidate = {
  id: string;
  name: string;
  email: string;
  role: string;
};

export type TribeRecord = {
  id: string;
  name: string;
};

export type ProjectRecord = {
  id: string;
  tribeId: string;
  name: string;
  repo: string;
  baseBranch: string;
};

export type AccountGitHub = {
  host: string;
  login: string;
  tokenSet: boolean;
  verified: boolean;
};

export type CompanyGitHub = {
  host: string;
  org: string;
  botLogin: string;
  tokenSet: boolean;
};

export type CompanyRepo = {
  name: string;
  defaultBranch: string;
  archived: boolean;
  private: boolean;
};

export type CompanyAccess = {
  ok: boolean;
  message?: string;
  login?: string;
  tokenKind?: 'classic' | 'fine-grained' | 'other';
  scopes?: string[];
  repoCount?: number;
  privateCount?: number;
  hint?: string;
};

export type Repository = {
  host: string;
  org: string;
  repo: string;
  baseBranch: string;
  cloned: boolean;
  headSha: string | null;
  canEdit: boolean;
};

export type ProjectLinks = { links: ProjectLink[]; canEdit: boolean };

export type ProjectLink = {
  projectId: string;
  name: string;
  repo: string;
  sharedPaths: string[];
};

export type Family = {
  id: string;
  protocol: string;
  baseUrl: string;
  models: string[];
  keySet: boolean;
};

export type LLMSettings = {
  families: Family[];
};

export type RouteChoice = {
  model: string;
  fallback: string;
};

export type RouterSettings = {
  routes: Record<string, RouteChoice>;
  budgets: {
    maxAttempts: number;
    maxModelTurnsPerAttempt: number;
    maxTokensPerCard: number;
    maxWallMinutesPerAttempt: number;
  };
  reviewIndependentPossible: boolean;
};

export type DiffLine = { kind: 'add' | 'del' | 'ctx' | 'meta'; text: string };

export type DiffFile = {
  path: string;
  added: number;
  deleted: number;
  binary: boolean;
  truncated: boolean;
  hunks: { header: string; lines: DiffLine[] }[];
};

export type CardDiff = {
  sha: string;
  base: string;
  compareUrl: string;
  missing: boolean;
  truncated: boolean;
  files: DiffFile[];
};

export type DeploySettings = {
  environment: string;
  devUrl: string;
  devHosts: string[];
  devCommand: string;
  rollbackCommand: string;
  healthUrl: string;
  deployTokenSet: boolean;
  canEdit: boolean;
  localCheck: string;
  devCheck: string;
  allowExecutables: string[];
  sandboxEngine: string;
  sandboxImage: string;
  implementationCap: number;
  prdPath: string;
  rfcPath: string;
  soloApproval: boolean;
};

export type RuleCategory = 'naming' | 'database' | 'style' | 'framework' | 'security' | 'testing';

export type ConventionRule = {
  id: string;
  scope: 'global' | 'stack';
  stackId: string;
  category: RuleCategory;
  title: string;
  body: string;
  version: number;
  contentHash: string;
  disabled: boolean;
  updatedBy: string | null;
  updatedAt: string;
};

export type StackMarkers = {
  files: string[];
  npmDeps: string[];
  pyDeps: string[];
};

export type Stack = {
  id: string;
  name: string;
  detect: StackMarkers;
  version: number;
  contentHash: string;
  disabled: boolean;
  updatedBy: string | null;
  updatedAt: string;
};

export type Skill = {
  id: string;
  slug: string;
  title: string;
  useWhen: string;
  body: string;
  version: number;
  contentHash: string;
  disabled: boolean;
  updatedBy: string | null;
  updatedAt: string;
};

export type ConventionCatalog = {
  rules: ConventionRule[];
  stacks: Stack[];
  skills: Skill[];
};

export type RuleInput = {
  scope: string;
  stackId: string;
  category: string;
  title: string;
  body: string;
  disabled: boolean;
};
export type StackInput = Pick<Stack, 'name' | 'detect' | 'disabled'>;
export type SkillInput = Pick<Skill, 'slug' | 'title' | 'useWhen' | 'body' | 'disabled'>;

export type ProjectConventions = {
  stacks: { id: string; name: string; source: 'detected' | 'manual' }[];
  skillIds: string[];
  rules: {
    id: string;
    title: string;
    scope: 'global' | 'stack';
    stackName: string;
    category: RuleCategory;
    version: number;
  }[];
  canEdit: boolean;
};
