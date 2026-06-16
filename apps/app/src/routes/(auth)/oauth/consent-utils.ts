export type ClientInfo = {
  name: string;
  clientId: string;
  redirectUrls: string[];
};

export function extractHost(uri: string | undefined): string {
  if (!uri) {
    return "";
  }
  try {
    return new URL(uri).host;
  } catch {
    return uri;
  }
}

export function resolveRedirectHost(
  clientInfo: ClientInfo | null | undefined,
  _queryParamUri: string | undefined
): string {
  // Always prefer the server-authoritative registered redirect URI over the
  // attacker-supplied query parameter. If the client record is unavailable
  // (unverified), return empty string — the warning alert already covers this.
  return extractHost(clientInfo?.redirectUrls[0]);
}
