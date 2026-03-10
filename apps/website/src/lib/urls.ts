const TRAILING_SLASH_REGEX = /\/$/;

export const dashboardHref = (path: string): string => {
  const base =
    process.env.NEXT_PUBLIC_DASHBOARD_URL?.replace(TRAILING_SLASH_REGEX, "") ??
    "";
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;
  return base ? `${base}${normalizedPath}` : normalizedPath;
};
