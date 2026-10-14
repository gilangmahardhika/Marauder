import { apiOrigin, unreachableMessage } from "$lib/origin";
import { session } from "$lib/session";
import type {
  AccountGitHub,
  AIReviewMode,
  AppNotification,
  Board,
  Card,
  CardContextChange,
  CardDiff,
  CompanyGitHub,
  CompanyAccess,
  CompanyRepo,
  ConventionCatalog,
  ConventionChoices,
  ConventionRule,
  DeploySettings,
  Executor,
  Intake,
  LLMSettings,
  MCPCatalogEntry,
  MCPCatalogPage,
  MCPCatalogSource,
  MCPDiscover,
  MCPServer,
  MCPServerInput,
  ProjectConventions,
  ProjectLink,
  ProjectLinks,
  ProjectRecord,
  Repository,
  RefineEvent,
  RefineRequest,
  RequirementDraft,
  RouterSettings,
  RuleInput,
  Runner,
  RunnerList,
  RunnerToken,
  SessionUser,
  Skill,
  SkillInput,
  Stack,
  StackInput,
  StreamEvent,
  TraceResponse,
  TraceStreamEvent,
  TribeCandidate,
  TribeConventions,
  TribeLayer,
  TribeMember,
  TribeRecord,
  UsageResponse,
} from "$lib/types";

const base = apiOrigin;

export { apiOrigin };

export class ApiError extends Error {
  status: number;
  code: string;
  fields: Record<string, string>;

  constructor(
    status: number,
    code: string,
    message: string,
    fields: Record<string, string> = {},
  ) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.fields = fields;
  }
}

type ErrorBody = {
  error?: { code?: string; message?: string; fields?: Record<string, string> };
};

/** Sends one request with the session's bearer. An abort rethrows as is; any other fetch failure is a network ApiError. */
async function send(path: string, init: RequestInit): Promise<Response> {
  const headers = new Headers(init.headers);
  const bearer = session.bearer();
  if (bearer) headers.set("Authorization", `Bearer ${bearer}`);
  if (init.body != null) headers.set("Content-Type", "application/json");
  let response: Response;
  try {
    response = await fetch(base + path, { ...init, headers });
  } catch (error) {
    if (init.signal?.aborted) throw error;
    throw new ApiError(0, "network", unreachableMessage);
  }
  if (response.status === 401 && path !== "/api/session") {
    session.clear();
    try {
      window.location.assign("/login");
    } catch {
      /* The test runner has no navigation. */
    }
  }
  return response;
}

function failure(response: Response, payload: ErrorBody): ApiError {
  const error = payload.error ?? {};
  return new ApiError(
    response.status,
    error.code ?? "error",
    error.message ?? response.statusText,
    error.fields ?? {},
  );
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await send(path, init);
  const text = await response.text();
  const payload = text ? (JSON.parse(text) as ErrorBody) : {};
  if (!response.ok) throw failure(response, payload);
  return payload as T;
}

/** Reads complete server-sent events from buffer and returns them with the unfinished rest. The event name is `kind`. */
export function parseEvents<T extends { kind: string } = RefineEvent>(
  buffer: string,
): {
  events: T[];
  rest: string;
} {
  const blocks = buffer.replace(/\r\n?/g, "\n").split("\n\n");
  const rest = blocks.pop() ?? "";
  const events: T[] = [];
  for (const block of blocks) {
    let kind = "";
    const data: string[] = [];
    for (const line of block.split("\n")) {
      if (line.startsWith("event:")) kind = line.slice(6).trim();
      else if (line.startsWith("data:"))
        data.push(line.slice(5).replace(/^ /, ""));
    }
    if (!kind || data.length === 0) continue;
    events.push({ ...JSON.parse(data.join("\n")), kind } as T);
  }
  return { events, rest };
}

/** Rejects with ApiError for a reply that is not a stream; otherwise passes each event to onEvent until the body ends. */
async function readStream<T extends { kind: string }>(
  response: Response,
  onEvent: (event: T) => void,
): Promise<void> {
  if (!response.ok || !response.body) {
    const text = await response.text();
    let payload: ErrorBody = {};
    try {
      payload = text ? (JSON.parse(text) as ErrorBody) : {};
    } catch {
      /* A proxy page is not JSON; the status text stands. */
    }
    throw failure(response, payload);
  }
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  for (;;) {
    const { done, value } = await reader.read();
    buffer += decoder.decode(value, { stream: !done });
    const parsed = parseEvents<T>(done ? buffer + "\n\n" : buffer);
    buffer = parsed.rest;
    for (const event of parsed.events) onEvent(event);
    if (done) return;
  }
}

export function openSession(email: string, password: string) {
  return request<{ bearer: string; expiresAt: string; user: SessionUser }>(
    "/api/session",
    {
      method: "POST",
      body: JSON.stringify({ email, password }),
    },
  );
}

export function closeSession() {
  return request<void>("/api/session", { method: "DELETE" });
}

type Me = SessionUser & {
  githubLogin: string;
  githubTokenSet: boolean;
  githubVerified: boolean;
  tribeAdminOf: string[];
};

export function getMe() {
  return request<Me>("/api/me");
}

export function putMe(body: { name: string }) {
  return request<Me>("/api/me", { method: "PUT", body: JSON.stringify(body) });
}

export function changePassword(currentPassword: string, newPassword: string) {
  return request<void>("/api/me/password", {
    method: "PUT",
    body: JSON.stringify({ currentPassword, newPassword }),
  });
}

export function getMyGitHub() {
  return request<AccountGitHub>("/api/me/github");
}

export function putMyGitHub(body: Record<string, unknown>) {
  return request<AccountGitHub>("/api/me/github", {
    method: "PUT",
    body: JSON.stringify(body),
  });
}

export function testMyGitHub() {
  return request<{ ok: boolean; login?: string; message?: string }>(
    "/api/me/github/test",
    {
      method: "POST",
      body: "{}",
    },
  );
}

function projectPrefix() {
  return `/api/projects/${session.project()}`;
}

export function getProjects() {
  return request<{ projects: ProjectRecord[] }>("/api/projects");
}

export function createProject(body: {
  tribeId: string;
  name: string;
  repo: string;
  baseBranch: string;
  links: { projectId: string; sharedPaths: string[] }[];
}) {
  return request<ProjectRecord & { links: ProjectLink[] }>("/api/projects", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function getTribes() {
  return request<{ tribes: TribeRecord[] }>("/api/tribes");
}

export function createTribe(name: string) {
  return request<TribeRecord>("/api/tribes", {
    method: "POST",
    body: JSON.stringify({ name }),
  });
}

export function addMember(tribeId: string, userId: string) {
  return request<void>(`/api/tribes/${tribeId}/members`, {
    method: "POST",
    body: JSON.stringify({ userId }),
  });
}

export function getMembers(tribeId: string) {
  return request<{ members: TribeMember[] }>(`/api/tribes/${tribeId}/members`);
}

export function setTribeAdmin(
  tribeId: string,
  userId: string,
  tribeAdmin: boolean,
) {
  return request<void>(`/api/tribes/${tribeId}/members/${userId}`, {
    method: "PUT",
    body: JSON.stringify({ tribeAdmin }),
  });
}

export function getTribeCandidates(tribeId: string) {
  return request<{ users: TribeCandidate[] }>(
    `/api/tribes/${tribeId}/candidates`,
  );
}

export function removeMember(tribeId: string, userId: string) {
  return request<void>(`/api/tribes/${tribeId}/members/${userId}`, {
    method: "DELETE",
  });
}

export function getMCPServers(tribeId: string) {
  return request<{ servers: MCPServer[] }>(`/api/tribes/${encodeURIComponent(tribeId)}/mcp-servers`);
}

export function createMCPServer(tribeId: string, body: MCPServerInput) {
  return request<MCPServer>(`/api/tribes/${encodeURIComponent(tribeId)}/mcp-servers`, {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function putMCPServer(tribeId: string, serverId: string, body: MCPServerInput) {
  return request<MCPServer>(
    `/api/tribes/${encodeURIComponent(tribeId)}/mcp-servers/${encodeURIComponent(serverId)}`,
    { method: "PUT", body: JSON.stringify(body) },
  );
}

export function deleteMCPServer(tribeId: string, serverId: string) {
  return request<void>(
    `/api/tribes/${encodeURIComponent(tribeId)}/mcp-servers/${encodeURIComponent(serverId)}`,
    { method: "DELETE" },
  );
}

export function discoverMCPTools(tribeId: string, serverId: string) {
  return request<MCPDiscover>(
    `/api/tribes/${encodeURIComponent(tribeId)}/mcp-servers/${encodeURIComponent(serverId)}/discover`,
    { method: "POST", body: "{}" },
  );
}

export function getMCPCatalog(
  tribeId: string,
  source: MCPCatalogSource,
  q: string,
  cursor = "",
  signal?: AbortSignal,
) {
  const params = new URLSearchParams({ source });
  if (q) params.set("q", q);
  if (cursor) params.set("cursor", cursor);
  return request<MCPCatalogPage>(`/api/tribes/${encodeURIComponent(tribeId)}/mcp-catalog?${params}`, { signal });
}

export async function getMCPCatalogEntry(tribeId: string, id: string): Promise<MCPCatalogEntry | null> {
  const page = await request<MCPCatalogPage>(
    `/api/tribes/${encodeURIComponent(tribeId)}/mcp-catalog?id=${encodeURIComponent(id)}`,
  );
  return page.entries[0] ?? null;
}

export function pullMCPImage(tribeId: string, image: string) {
  return request<{ image: string }>(`/api/tribes/${encodeURIComponent(tribeId)}/mcp-images/pull`, {
    method: "POST",
    body: JSON.stringify({ image }),
  });
}

export function getUsers() {
  return request<{ users: SessionUser[] }>("/api/users");
}

export function getUsage(from: string, to: string) {
  const query = new URLSearchParams({ from, to });
  return request<UsageResponse>(`/api/usage?${query}`);
}

export function getTribeUsage(id: string, from: string, to: string) {
  const query = new URLSearchParams({ from, to });
  return request<UsageResponse>(`/api/tribes/${encodeURIComponent(id)}/usage?${query}`);
}

export function getMyUsage(from: string, to: string) {
  const query = new URLSearchParams({ from, to });
  return request<UsageResponse>(`/api/me/usage?${query}`);
}

export function createUser(body: {
  name: string;
  email: string;
  password: string;
  role: string;
  admin: boolean;
}) {
  return request<SessionUser>("/api/users", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function updateUser(
  id: string,
  body: {
    name?: string;
    role: string;
    admin: boolean;
    disabled: boolean;
    password?: string;
  },
) {
  return request<SessionUser>(`/api/users/${id}`, {
    method: "PUT",
    body: JSON.stringify(body),
  });
}

export function getCards() {
  return request<Board>(`${projectPrefix()}/cards`);
}

export function getCard(id: string) {
  return request<Card>(`${projectPrefix()}/cards/${id}`);
}

export function getCardDiff(id: string) {
  return request<CardDiff>(`${projectPrefix()}/cards/${id}/diff`);
}

/** With `since`, the server returns events from 10 seconds before it; merge them by id. */
export function getTrace(id: string, since?: string) {
  const query = since ? `?since=${encodeURIComponent(since)}` : "";
  return request<TraceResponse>(`${projectPrefix()}/cards/${id}/trace${query}`);
}

/**
 * Streams the card's trace while it works (RFC-088): stored events as they commit, and the model's reply as it is
 * written. It resolves when the server ends the stream and rejects with ApiError, or when signal aborts. The bearer
 * goes in a header, so this reads the fetch body instead of using EventSource.
 */
export async function streamTrace(
  id: string,
  since: string | undefined,
  onEvent: (event: TraceStreamEvent) => void,
  signal: AbortSignal,
): Promise<void> {
  const query = since ? `?since=${encodeURIComponent(since)}` : "";
  const response = await send(
    `${projectPrefix()}/cards/${id}/trace/stream${query}`,
    { signal },
  );
  await readStream(response, onEvent);
}

/**
 * Opens the push stream (RFC-113): the project's card changes with this viewer's actions when project is set, and the
 * viewer's notifications. It resolves when the server ends the stream and rejects with ApiError, or when signal aborts.
 */
export async function openStream(
  project: string,
  onEvent: (event: StreamEvent) => void,
  signal: AbortSignal,
): Promise<void> {
  const query = project ? `?project=${encodeURIComponent(project)}` : "";
  const response = await send(`/api/stream${query}`, { signal });
  await readStream(response, onEvent);
}

export function getNotifications(unread = false) {
  return request<{ notifications: AppNotification[]; unread: number }>(
    `/api/notifications${unread ? "?unread=1" : ""}`,
  );
}

export function readNotifications(body: { ids: string[] } | { all: true }) {
  return request<{ unread: number }>("/api/notifications/read", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function getPushKey() {
  return request<{ publicKey: string; subscriptions: number }>("/api/push/key");
}

export function savePushSubscription(subscription: PushSubscriptionJSON) {
  return request<void>("/api/push/subscriptions", {
    method: "POST",
    body: JSON.stringify({ endpoint: subscription.endpoint, keys: subscription.keys }),
  });
}

export function deletePushSubscription(endpoint: string) {
  return request<void>("/api/push/subscriptions", {
    method: "DELETE",
    body: JSON.stringify({ endpoint }),
  });
}

export function testPush() {
  return request<{ sent: number }>("/api/push/test", { method: "POST", body: "{}" });
}

export function getIntake() {
  return request<Intake>(`${projectPrefix()}/intake`);
}

export function ingest() {
  return request<Intake>(`${projectPrefix()}/ingest`, {
    method: "POST",
    body: "{}",
  });
}

export function acceptRequirements(requirementIds: string[]) {
  return request<Intake>(`${projectPrefix()}/accept`, {
    method: "POST",
    body: JSON.stringify({ requirementIds }),
  });
}

export function addRequirement(
  body: RequirementDraft & {
    id: string;
    selfReview?: boolean;
    aiReviewMode?: AIReviewMode;
  },
) {
  return request<Intake>(`${projectPrefix()}/requirements`, {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function updateRequirement(id: string, body: RequirementDraft) {
  return request<Intake>(
    `${projectPrefix()}/requirements/${encodeURIComponent(id)}`,
    {
      method: "PUT",
      body: JSON.stringify(body),
    },
  );
}

/**
 * Streams the implement model's new text for one field of a draft (RFC-087). requirementId is empty for a new
 * requirement. Nothing is saved. EventSource cannot send the bearer, so this reads the fetch body.
 */
export async function streamRefine(
  body: RefineRequest & { requirementId: string },
  onEvent: (event: RefineEvent) => void,
  signal: AbortSignal,
): Promise<void> {
  const response = await send(`${projectPrefix()}/requirements/refine`, {
    method: "POST",
    body: JSON.stringify(body),
    signal,
  });
  await readStream(response, onEvent);
}

export function setReviewers(id: string, userIds: string[]) {
  return request<Intake>(
    `${projectPrefix()}/requirements/${encodeURIComponent(id)}/reviewers`,
    {
      method: "PUT",
      body: JSON.stringify({ userIds }),
    },
  );
}

export function setSelfReview(id: string, selfReview: boolean) {
  return request<Intake>(
    `${projectPrefix()}/requirements/${encodeURIComponent(id)}/self-review`,
    {
      method: "PUT",
      body: JSON.stringify({ selfReview }),
    },
  );
}

export function setAIReviewMode(id: string, aiReviewMode: AIReviewMode) {
  return request<Intake>(
    `${projectPrefix()}/requirements/${encodeURIComponent(id)}/ai-review-mode`,
    {
      method: "PUT",
      body: JSON.stringify({ aiReviewMode }),
    },
  );
}

export function deleteRequirement(id: string) {
  return request<Intake>(
    `${projectPrefix()}/requirements/${encodeURIComponent(id)}`,
    { method: "DELETE" },
  );
}

export function startRun() {
  return request<{ queued: number; readyCount: number }>(
    `${projectPrefix()}/runs`,
    {
      method: "POST",
      body: "{}",
    },
  );
}

export function runCard(id: string, instructions?: string) {
  const body = instructions === undefined ? {} : { instructions };
  return request<Card>(`${projectPrefix()}/cards/${id}/run`, {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function setInstructions(id: string, instructions: string) {
  return request<Card>(`${projectPrefix()}/cards/${id}/instructions`, {
    method: "PUT",
    body: JSON.stringify({ instructions }),
  });
}

export function setCardContext(id: string, change: CardContextChange) {
  return request<Card>(`${projectPrefix()}/cards/${id}/context`, {
    method: "PUT",
    body: JSON.stringify(change),
  });
}

export function deployCard(id: string) {
  return request<Card>(`${projectPrefix()}/cards/${id}/deploy`, {
    method: "POST",
    body: "{}",
  });
}

export function mergeCard(id: string) {
  return request<Card>(`${projectPrefix()}/cards/${id}/merge`, {
    method: "POST",
    body: "{}",
  });
}

export function wontFixCard(id: string, reason: string) {
  return request<Card>(`${projectPrefix()}/cards/${id}/wont-fix`, {
    method: "POST",
    body: JSON.stringify({ reason }),
  });
}

export function reopenCard(id: string) {
  return request<Card>(`${projectPrefix()}/cards/${id}/reopen`, {
    method: "POST",
    body: "{}",
  });
}

export function retryReview(id: string) {
  return request<Card>(`${projectPrefix()}/cards/${id}/retry-review`, {
    method: "POST",
    body: "{}",
  });
}

export function approveCard(id: string, sha: string) {
  return request<Card>(`${projectPrefix()}/cards/${id}/approve`, {
    method: "POST",
    body: JSON.stringify({ sha }),
  });
}

export function requestChanges(id: string, reason = "") {
  return request<Card>(`${projectPrefix()}/cards/${id}/request-changes`, {
    method: "POST",
    body: JSON.stringify({ reason }),
  });
}

export function rejectCard(id: string) {
  return request<Card>(`${projectPrefix()}/cards/${id}/reject`, {
    method: "POST",
    body: "{}",
  });
}

export function unblockCard(id: string, resetAttempts: boolean) {
  return request<Card>(`${projectPrefix()}/cards/${id}/unblock`, {
    method: "POST",
    body: JSON.stringify({ resetAttempts }),
  });
}

export function getCompanyGitHub() {
  return request<CompanyGitHub>("/api/settings/github");
}

export function putCompanyGitHub(body: Record<string, unknown>) {
  return request<CompanyGitHub>("/api/settings/github", {
    method: "PUT",
    body: JSON.stringify(body),
  });
}

export function testCompanyGitHub() {
  return request<CompanyAccess>("/api/settings/github/test", {
    method: "POST",
    body: "{}",
  });
}

export function getCompanyRepos() {
  return request<{ repos: CompanyRepo[] }>("/api/settings/github/repos");
}

export function getRepository() {
  return request<Repository>(`${projectPrefix()}/settings/repository`);
}

export function putRepository(body: { repo: string; baseBranch: string }) {
  return request<Repository>(`${projectPrefix()}/settings/repository`, {
    method: "PUT",
    body: JSON.stringify(body),
  });
}

export function syncRepository() {
  return request<Repository>(`${projectPrefix()}/sync`, {
    method: "POST",
    body: "{}",
  });
}

export function getLinks() {
  return request<ProjectLinks>(`${projectPrefix()}/settings/links`);
}

export function putLinks(
  links: { projectId: string; sharedPaths: string[] }[],
) {
  return request<ProjectLinks>(`${projectPrefix()}/settings/links`, {
    method: "PUT",
    body: JSON.stringify({ links }),
  });
}

export function getLLM() {
  return request<LLMSettings>("/api/settings/llm");
}

export function putLLM(body: Record<string, unknown>) {
  return request<LLMSettings>("/api/settings/llm", {
    method: "PUT",
    body: JSON.stringify(body),
  });
}

export function testLLM(family: string) {
  return request<{ ok: boolean; message?: string }>(
    `/api/settings/llm/${family}/test`,
    {
      method: "POST",
      body: "{}",
    },
  );
}

export function getRouter() {
  return request<RouterSettings>("/api/settings/router");
}

export function putRouter(body: Record<string, unknown>) {
  return request<RouterSettings>("/api/settings/router", {
    method: "PUT",
    body: JSON.stringify(body),
  });
}

export function getRunners() {
  return request<RunnerList>("/api/runners");
}

export function runnerAction(id: string, action: string) {
  return request<Runner | Record<string, never>>(
    `/api/runners/${encodeURIComponent(id)}/${encodeURIComponent(action)}`,
    { method: "POST", body: "{}" },
  );
}

export function mintRunnerToken() {
  return request<RunnerToken>("/api/runners/tokens", {
    method: "POST",
    body: "{}",
  });
}

export function putJobs(executor: Executor) {
  return request<{ executor: Executor }>("/api/settings/jobs", {
    method: "PUT",
    body: JSON.stringify({ executor }),
  });
}

export function getDeploy() {
  return request<DeploySettings>(`${projectPrefix()}/settings/deploy`);
}

export function putDeploy(body: Record<string, unknown>) {
  return request<DeploySettings>(`${projectPrefix()}/settings/deploy`, {
    method: "PUT",
    body: JSON.stringify(body),
  });
}

export function getConventions() {
  return request<ConventionCatalog>("/api/conventions");
}

export type ConventionKind = "rules" | "stacks" | "skills";

function saveConvention<T>(kind: ConventionKind, id: string, body: unknown) {
  const path = id
    ? `/api/conventions/${kind}/${encodeURIComponent(id)}`
    : `/api/conventions/${kind}`;
  return request<T>(path, {
    method: id ? "PUT" : "POST",
    body: JSON.stringify(body),
  });
}

export function saveRule(id: string, body: RuleInput) {
  return saveConvention<ConventionRule>("rules", id, body);
}

export function saveStack(id: string, body: StackInput) {
  return saveConvention<Stack>("stacks", id, body);
}

export function saveSkill(id: string, body: SkillInput) {
  return saveConvention<Skill>("skills", id, body);
}

export function deleteConvention(kind: ConventionKind, id: string) {
  return request<void>(`/api/conventions/${kind}/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export function getProjectConventions() {
  return request<ProjectConventions>(`${projectPrefix()}/settings/conventions`);
}

export function putProjectConventions(body: ConventionChoices) {
  return request<ProjectConventions>(
    `${projectPrefix()}/settings/conventions`,
    {
      method: "PUT",
      body: JSON.stringify(body),
    },
  );
}

export function getTribeConventions(tribeId: string) {
  return request<TribeConventions>(
    `/api/tribes/${encodeURIComponent(tribeId)}/conventions`,
  );
}

export function putTribeConventions(tribeId: string, body: TribeLayer) {
  return request<TribeConventions>(
    `/api/tribes/${encodeURIComponent(tribeId)}/conventions`,
    {
      method: "PUT",
      body: JSON.stringify(body),
    },
  );
}

export function unreachable() {
  return unreachableMessage;
}
