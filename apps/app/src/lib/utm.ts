const UTM_STORAGE_KEY = "strait_utm_params";

export type UtmParams = {
  utm_source?: string;
  utm_medium?: string;
  utm_campaign?: string;
  utm_term?: string;
  utm_content?: string;
  ref?: string;
};

export const storeUtmParams = (params: UtmParams) => {
  const filtered = Object.fromEntries(
    Object.entries(params).filter(([, v]) => v != null && v !== "")
  );
  if (Object.keys(filtered).length === 0) {
    return;
  }
  try {
    sessionStorage.setItem(UTM_STORAGE_KEY, JSON.stringify(filtered));
  } catch {
    // sessionStorage may be unavailable in SSR or private browsing
  }
};

export const consumeUtmParams = (): UtmParams | null => {
  try {
    const raw = sessionStorage.getItem(UTM_STORAGE_KEY);
    if (!raw) {
      return null;
    }
    sessionStorage.removeItem(UTM_STORAGE_KEY);
    return JSON.parse(raw) as UtmParams;
  } catch {
    return null;
  }
};

export const utmToSetOnce = (params: UtmParams): Record<string, string> => {
  const mapping: [keyof UtmParams, string][] = [
    ["utm_source", "initial_utm_source"],
    ["utm_medium", "initial_utm_medium"],
    ["utm_campaign", "initial_utm_campaign"],
    ["utm_term", "initial_utm_term"],
    ["utm_content", "initial_utm_content"],
    ["ref", "initial_referrer"],
  ];
  const result: Record<string, string> = {};
  for (const [key, prop] of mapping) {
    if (params[key]) {
      result[prop] = params[key];
    }
  }
  return result;
};
