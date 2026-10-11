import { beforeEach, expect, test, vi } from "vitest";
import {
  createProject,
  getCards,
  getTribeCandidates,
  putDeploy,
  putLinks,
  setTribeAdmin,
  syncRepository,
} from "./api";
import { session } from "./session";

beforeEach(() => {
  session.set("stored-bearer");
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            error: { code: "unauthenticated", message: "bearer" },
          }),
          { status: 401 },
        ),
    ),
  );
});

test("401 clears the bearer", async () => {
  await expect(getCards()).rejects.toThrow(/bearer/);
  expect(session.bearer()).toBe("");
});

test("project settings use the selected project", async () => {
  const calls: { url: string; body: string }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, body: String(init.body ?? "") });
      return new Response("{}", { status: 200 });
    }),
  );
  session.setProject("proj_widget");
  await putDeploy({ environment: "dev" });
  await syncRepository();
  await putLinks([{ projectId: "proj_api", sharedPaths: ["docs/api"] }]);
  await createProject({
    tribeId: "tribe_a",
    name: "Widget",
    repo: "widget",
    baseBranch: "main",
    links: [{ projectId: "proj_api", sharedPaths: [] }],
  });
  expect(calls.map((call) => call.url)).toEqual([
    "http://127.0.0.1:8787/api/projects/proj_widget/settings/deploy",
    "http://127.0.0.1:8787/api/projects/proj_widget/sync",
    "http://127.0.0.1:8787/api/projects/proj_widget/settings/links",
    "http://127.0.0.1:8787/api/projects",
  ]);
  expect(JSON.parse(calls[2].body)).toEqual({
    links: [{ projectId: "proj_api", sharedPaths: ["docs/api"] }],
  });
  expect(JSON.parse(calls[3].body)).toEqual({
    tribeId: "tribe_a",
    name: "Widget",
    repo: "widget",
    baseBranch: "main",
    links: [{ projectId: "proj_api", sharedPaths: [] }],
  });
});

test("tribe admin calls use the tribe and member ids", async () => {
  const calls: { url: string; method: string; body: string }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({
        url,
        method: String(init.method ?? "GET"),
        body: String(init.body ?? ""),
      });
      return new Response(init.method === "PUT" ? null : '{"users":[]}', {
        status: init.method === "PUT" ? 204 : 200,
      });
    }),
  );
  await setTribeAdmin("tribe_a", "user_b", true);
  await getTribeCandidates("tribe_a");
  expect(calls.map((call) => `${call.method} ${call.url}`)).toEqual([
    "PUT http://127.0.0.1:8787/api/tribes/tribe_a/members/user_b",
    "GET http://127.0.0.1:8787/api/tribes/tribe_a/candidates",
  ]);
  expect(JSON.parse(calls[0].body)).toEqual({ tribeAdmin: true });
});
