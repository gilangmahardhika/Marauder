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
  /** Single card only. The rules, skills, and MCP servers the next attempt carries (RFC-105). */
  context?: CardContext;
  /** Single card only. The pull request text the card last published, `null` before it publishes (RFC-110). */
  pullRequest?: CardPullRequest | null;
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
  reviewVerdict: "recommend_approve" | "request_changes" | "failed" | null;
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

/** The title and harness description of the published pull request, without the metadata block. */
export type CardPullRequest = {
  title: string;
  body: string;
  attemptN: number;
  sha: string;
};

export type CardContextRule = EffectiveRule & { state: "on" | "off" };
export type CardContextSkill = EffectiveSkill & { state: "on" | "off" };
/** A server of the card's tribe: no transport, host, or secret (RFC-101). */
export type CardContextServer = {
  id: string;
  slug: string;
  enabled: boolean;
  toolCount: number;
  state: "on" | "off";
  origin: "scope" | "card";
};
export type CardContext = {
  rules: CardContextRule[];
  skills: CardContextSkill[];
  mcp: CardContextServer[];
  mcpScope: "all" | "mentioned" | "card" | "none";
};
export type CardContextChange = {
  kind: "skill" | "rule" | "mcp";
  itemId: string;
  mode: "attach" | "detach" | "inherit";
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
  /** Insert order; it breaks ties between events in the same second. */
  seq?: number;
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
  /** 0 when the attempt has no turn budget. */
  maxTurns?: number;
  commitSha: string;
  reviewModelId: string;
  reviewVerdict: string;
  reviewIndependent: boolean | null;
  startedAt: string;
  endedAt: string;
  outcome: string;
};

/** `live` is true while the card is queued or working, so more events may come. */
export type TraceResponse = {
  attempts: TraceAttempt[];
  events: TraceEvent[];
  live?: boolean;
};

/**
 * One frame of the trace stream (RFC-088). `trace` carries stored events since the last frame; `turn`, `delta`,
 * `reset`, and `turn_end` carry the model's reply while it is written, which is never stored. `done` ends the stream:
 * `live: false` when the card stopped working, `reconnect: true` when the page should connect again. `resync` means the
 * server dropped a viewer that fell behind.
 */
export type TraceStreamEvent =
  | ({ kind: "trace" } & TraceResponse)
  | {
      kind: "turn";
      attemptId: string;
      turn: number;
      role: string;
      modelId?: string;
    }
  | { kind: "delta"; attemptId: string; turn: number; text?: string }
  | { kind: "reset"; attemptId: string; turn: number }
  | { kind: "turn_end"; attemptId: string; turn: number }
  | { kind: "done"; live?: boolean; reconnect?: boolean }
  | { kind: "resync" };

export type Requirement = {
  id: string;
  sourceKind: "prd" | "rfc" | "draft" | "manual";
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
  /** The tribe MCP servers the requirement names with @slug (RFC-101). */
  mentions?: string[];
};

/** A tribe MCP server as a project member sees it, to mention it (RFC-101). */
export type MCPServerMention = {
  slug: string;
  enabled: boolean;
  toolCount: number;
};

export type AIReviewMode = "continue" | "manual";

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

/** The field a refine rewrites (RFC-087). */
export type RefineField = "statement" | "acceptance";

export type RefineRequest = RequirementDraft & {
  field: RefineField;
  instructions: string;
};

/**
 * One server-sent event of a streamed refine. done.text is the whole new text; for acceptance it is the lines joined by newlines.
 * reset clears the text so far, because that turn ended in MCP tool calls; tool names each call's server and tool.
 */
export type RefineEvent =
  | { kind: "start"; modelId: string }
  | { kind: "delta"; text: string }
  | { kind: "reset" }
  | { kind: "tool"; server: string; tool: string }
  | { kind: "done"; text: string; acceptance?: string[]; modelId: string }
  | { kind: "error"; code: string; message: string };

/** Streams one refine. It resolves when the stream ends and rejects with ApiError for an error before the stream, or when signal aborts. */
export type RefineStream = (
  body: RefineRequest,
  onEvent: (event: RefineEvent) => void,
  signal: AbortSignal,
) => Promise<void>;

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
  mcpServers?: MCPServerMention[];
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
  tokenKind?: "classic" | "fine-grained" | "other";
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

export type ModelOptions = {
  thinking?: string;
  thinkingBudget?: number;
  effort?: string;
  historyBudgetKiB?: number;
};

export type Family = {
  id: string;
  protocol: string;
  baseUrl: string;
  models: string[];
  modelOptions?: Record<string, ModelOptions>;
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

export type Executor = "local" | "runners";

export type RunnerStatus = "idle" | "busy" | "draining" | "offline";

export type RunnerLease = {
  id: string;
  cardId: string;
  projectId: string;
  since: string;
};

export type Runner = {
  id: string;
  name: string;
  labels: string[];
  slots: number;
  busy: number;
  status: RunnerStatus;
  version: string;
  protocol: number;
  updateRequired: boolean;
  heartbeatAt: string | null;
  leases: RunnerLease[];
  actions: string[];
};

export type RunnerList = {
  executor: Executor;
  protocol: number;
  runners: Runner[];
};

export type RunnerToken = { token: string; expiresAt: string };

export type DiffLine = { kind: "add" | "del" | "ctx" | "meta"; text: string };

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
  commandExecutables: string[];
  sandboxEngine: string;
  sandboxImage: string;
  implementationCap: number;
  prdPath: string;
  rfcPath: string;
  soloApproval: boolean;
};

export type RuleCategory =
  "naming" | "database" | "style" | "framework" | "security" | "testing";

export type ConventionRule = {
  id: string;
  scope: "global" | "stack";
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
export type StackInput = Pick<Stack, "name" | "detect" | "disabled">;
export type SkillInput = Pick<
  Skill,
  "slug" | "title" | "useWhen" | "body" | "disabled"
>;

/** The layer that put a convention in a set (RFC-104, RFC-105). */
export type ConventionOrigin = "catalog" | "detected" | "tribe" | "project" | "planner" | "card";

/** What a tribe sets for all its projects: stacks and skills on, rules off (RFC-104). */
export type TribeLayer = { stackIds: string[]; skillIds: string[]; rulesOff: string[] };
export type TribeConventions = TribeLayer & { canEdit: boolean };

export type OnOff = { on: string[]; off: string[] };
export type ConventionChoices = { stacks: OnOff; skills: OnOff; rules: OnOff };

export type EffectiveRule = {
  id: string;
  title: string;
  scope: "global" | "stack";
  stackName: string;
  category: RuleCategory;
  version: number;
  origin: ConventionOrigin;
};

export type EffectiveSkill = {
  id: string;
  slug: string;
  title: string;
  useWhen: string;
  version: number;
  origin: ConventionOrigin;
};

export type ProjectConventions = {
  tribe: TribeLayer;
  choices: ConventionChoices;
  detectedStackIds: string[];
  stacks: { id: string; name: string; origin: ConventionOrigin }[];
  skills: EffectiveSkill[];
  rules: EffectiveRule[];
  canEdit: boolean;
};

/**
 * One day of model calls for one project, model, task type, user, source, and card (RFC-089, RFC-091). `familyId` is
 * empty for a model no LLM family lists; `userId` is empty for a card with no owner. A refine has `source` `intake`
 * and no card. Total tokens are input plus output.
 */
export type UsageRow = {
  day: string;
  projectId: string;
  projectName: string;
  familyId: string;
  protocol: string;
  modelId: string;
  task: string;
  userId: string;
  userName: string;
  userEmail: string;
  source: "intake" | "card";
  cardId: string;
  cardTitle: string;
  calls: number;
  inputTokens: number;
  outputTokens: number;
  cachedInputTokens: number;
};

/** `from` and `to` are inclusive YYYY-MM-DD days in UTC. */
export type UsageResponse = { from: string; to: string; rows: UsageRow[] };

/** One allowlisted tool of a tribe MCP server. `description` is the admin's text (RFC-092). */
export type MCPTool = {
  name: string;
  description: string;
  schema: Record<string, unknown>;
  schemaSha256: string;
};

export type MCPTransport = "http" | "stdio" | "postgres" | "sqlserver";

/** A database connector's connection (RFC-097). The password is a secret named PASSWORD, never part of it. */
export type MCPDBConfig = {
  host: string;
  port: number;
  database: string;
  user: string;
  tlsMode: "disable" | "require" | "verify-full";
  readOnlyLogin: boolean;
};

/**
 * A tribe's MCP server (RFC-092, RFC-093, RFC-097). `secretsSet` names the headers (http), environment variables
 * (stdio), or PASSWORD (a database) that hold a value; the values are never returned.
 */
export type MCPServer = {
  id: string;
  tribeId: string;
  slug: string;
  transport: MCPTransport;
  url: string;
  image: string;
  argv: string[];
  config: MCPDBConfig | null;
  enabled: boolean;
  secretsSet: string[];
  tools: MCPTool[];
  updatedAt: string;
};

/** A save. A `secrets` entry with an empty value keeps the stored value; `clear` deletes values. */
export type MCPServerInput = {
  slug: string;
  transport: MCPTransport;
  url: string;
  image: string;
  argv: string[];
  config: MCPDBConfig | null;
  enabled: boolean;
  secrets: Record<string, string>;
  clear: string[];
  tools: { name: string; description: string; schema: Record<string, unknown> }[];
};

/** A tool the server lists. `valid` is false for a name the allowlist cannot take. */
export type MCPDiscoveredTool = MCPTool & { valid: boolean };

/** Discovery. `message` is a fixed reason when `ok` is false. */
export type MCPDiscover = { ok: boolean; message?: string; tools: MCPDiscoveredTool[] };

/** A secret a catalog entry needs: a header, an environment variable, or PASSWORD. */
export type MCPCatalogSecret = { name: string; description: string; required: boolean };

/**
 * A server the admin can set up from the catalog (RFC-096, RFC-098, RFC-099). `supported` is false with a `reason`
 * when the harness cannot run it. Built-in database entries carry their fixed `tools` and default `port`.
 */
export type MCPCatalogEntry = {
  id: string;
  source: "builtin" | "featured" | "registry";
  name: string;
  title: string;
  description: string;
  websiteUrl: string;
  version: string;
  transport: MCPTransport;
  supported: boolean;
  reason?: string;
  note?: string;
  url?: string;
  image?: string;
  argv: string[];
  port?: number;
  secrets: MCPCatalogSecret[];
  tools: { name: string; description: string; schema: Record<string, unknown> }[];
};

/** One catalog page. `registryError` is a fixed notice when the registry could not be read. */
export type MCPCatalogPage = { entries: MCPCatalogEntry[]; nextCursor: string; registryError?: string };

/** `local` is the built-in and featured entries, answered at once; `registry` is one MCP Registry page (RFC-100). */
export type MCPCatalogSource = "local" | "registry";
