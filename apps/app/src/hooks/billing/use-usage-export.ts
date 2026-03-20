import { createServerFn } from "@tanstack/react-start";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { getOrgIdFromSession } from "./session";

type ExportInput = {
  period: string;
};

const getUsageExportServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: ExportInput) => data)
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
        params: {
          org_id: orgId,
          period: data.period,
          format: "csv",
        },
        responseType: "text",
      })
    );
  });

export function fetchUsageExportCsv(period: string): Promise<string> {
  return getUsageExportServerFn({ data: { period } });
}
