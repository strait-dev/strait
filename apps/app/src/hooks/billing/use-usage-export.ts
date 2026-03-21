import { createServerFn } from "@tanstack/react-start";
import z from "zod/v4";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { getOrgIdFromSession } from "./session";

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
    return await runWithSentryReport(
      apiEffect<string>("/v1/usage/export", {
        params: { org_id: orgId, period: data.period, format: "csv" },
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
    const buffer = await runWithSentryReport(
      apiEffect<string>("/v1/usage/export", {
        params: { org_id: orgId, period: data.period, format: "pdf" },
        responseType: "text",
      })
    );
    // Encode binary response as base64 for serialization across server/client boundary
    return Buffer.from(buffer, "binary").toString("base64");
  });

export function fetchUsageExportCsv(period: string): Promise<string> {
  return getUsageExportCsvServerFn({ data: { period } });
}

export function fetchUsageExportPdf(period: string): Promise<string | null> {
  return getUsageExportPdfServerFn({ data: { period } });
}
