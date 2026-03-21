import { createServerFn } from "@tanstack/react-start";
import z from "zod/v4";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { getOrgIdFromSession } from "./session";

/** Convert a "YYYY-MM" period string to from/to date range params. */
function periodToDateRange(period: string): { from: string; to: string } {
  const [year, month] = period.split("-").map(Number);
  const from = `${year}-${String(month).padStart(2, "0")}-01`;
  const lastDay = new Date(year, month, 0).getDate();
  const to = `${year}-${String(month).padStart(2, "0")}-${String(lastDay).padStart(2, "0")}`;
  return { from, to };
}

const getUsageExportCsvServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: { period: string }) =>
    z.object({ period: z.string().min(1) }).parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ data, context }) => {
    const orgId = getOrgIdFromSession(
      context.session as Record<string, unknown>
    );
    if (!orgId) {
      return "";
    }
    const { from, to } = periodToDateRange(data.period);
    return await runWithSentryReport(
      apiEffect<string>("/v1/usage/export", {
        params: { org_id: orgId, from, to, format: "csv" },
        responseType: "text",
      })
    );
  });

const getUsageExportPdfServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: { period: string }) =>
    z.object({ period: z.string().min(1) }).parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ data, context }) => {
    const orgId = getOrgIdFromSession(
      context.session as Record<string, unknown>
    );
    if (!orgId) {
      return null;
    }
    const { from, to } = periodToDateRange(data.period);
    const buffer = await runWithSentryReport(
      apiEffect<string>("/v1/usage/export", {
        params: { org_id: orgId, from, to, format: "pdf" },
        responseType: "text",
      })
    );
    return Buffer.from(buffer, "binary").toString("base64");
  });

export function fetchUsageExportCsv(period: string): Promise<string> {
  return getUsageExportCsvServerFn({ data: { period } });
}

export function fetchUsageExportPdf(period: string): Promise<string | null> {
  return getUsageExportPdfServerFn({ data: { period } });
}
