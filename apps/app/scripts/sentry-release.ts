export type ReleaseEnv = Record<string, string | undefined> & {
  SENTRY_RELEASE?: string;
  STRAIT_VERSION?: string;
  STRAIT_COMMIT?: string;
};

export function resolveSentryRelease(env: ReleaseEnv): string {
  if (env.SENTRY_RELEASE?.trim()) {
    return env.SENTRY_RELEASE.trim();
  }

  const version = env.STRAIT_VERSION?.trim();
  if (!version) {
    throw new Error("SENTRY_RELEASE or STRAIT_VERSION must be set");
  }

  const commit = env.STRAIT_COMMIT?.trim();
  if (!commit || commit === "none" || commit === "unknown") {
    return version;
  }

  return `${version}+${shortCommit(commit)}`;
}

function shortCommit(commit: string): string {
  return commit.length <= 12 ? commit : commit.slice(0, 12);
}
