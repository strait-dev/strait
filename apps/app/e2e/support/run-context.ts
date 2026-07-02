import fs from "node:fs";

export const e2eAuthDir = "playwright/.auth";
export const projectContextPath = `${e2eAuthDir}/project.json`;
export const fakeEndpointContextPath = `${e2eAuthDir}/fake-endpoint.json`;

export type E2ERunContext = {
  projectId?: string;
  orgId?: string;
  userId?: string;
  limitedUserId?: string;
  limitedUserEmail?: string;
  fakeEndpointUrl?: string;
  fakeEndpointPid?: number;
  managedFakeEndpoint?: boolean;
};

/** Read shared e2e runtime context created by Playwright global setup. */
export function readRunContext(): E2ERunContext | null {
  return readJson<E2ERunContext>(projectContextPath);
}

/** Merge setup values into the shared e2e context file. */
export function writeRunContext(context: E2ERunContext) {
  fs.mkdirSync(e2eAuthDir, { recursive: true });
  const current = readRunContext() ?? {};
  fs.writeFileSync(
    projectContextPath,
    JSON.stringify({ ...current, ...context })
  );
}

export type FakeEndpointContext = {
  url: string;
  pid?: number;
  managed: boolean;
};

export function readFakeEndpointContext() {
  return readJson<FakeEndpointContext>(fakeEndpointContextPath);
}

export function writeFakeEndpointContext(context: FakeEndpointContext) {
  fs.mkdirSync(e2eAuthDir, { recursive: true });
  fs.writeFileSync(fakeEndpointContextPath, JSON.stringify(context));
  writeRunContext({
    fakeEndpointUrl: context.url,
    fakeEndpointPid: context.pid,
    managedFakeEndpoint: context.managed,
  });
}

function readJson<T>(path: string): T | null {
  try {
    return JSON.parse(fs.readFileSync(path, "utf-8")) as T;
  } catch {
    return null;
  }
}
