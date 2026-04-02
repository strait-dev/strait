/**
 * Usage data export server functions.
 *
 * Fetches CSV and PDF usage reports from `GET /v1/usage/export`.
 * Used by the usage history tab's export buttons.
 */

import { createServerFn } from "@tanstack/react-start";
import z from "zod/v4";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { periodToDateRange } from "./period-utils";
import { getOrgIdFromSession } from "./session";

/** Server function to fetch a CSV usage export. */
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

/** Server function to fetch a PDF usage export as base64. */
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

/**
 * Fetch a CSV usage export for the given billing period.
 *
 * @param period - Billing period in "YYYY-MM" format (e.g. "2026-03").
 * @returns The CSV content as a string.
 */
export const fetchUsageExportCsv = (period: string): Promise<string> =>
  getUsageExportCsvServerFn({ data: { period } });

/**
 * Fetch a PDF usage export for the given billing period as base64.
 *
 * @param period - Billing period in "YYYY-MM" format (e.g. "2026-03").
 * @returns The PDF content as a base64-encoded string, or `null` if no org is active.
 */
export const fetchUsageExportPdf = (period: string): Promise<string | null> =>
  getUsageExportPdfServerFn({ data: { period } });
