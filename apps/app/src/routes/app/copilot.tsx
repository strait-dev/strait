import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Shell } from "@strait/ui/components/shell";
import { Textarea } from "@strait/ui/components/textarea";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";

import {
  answerCopilotPrompt,
  buildSuggestedPrompts,
} from "@/components/agents/copilot-utils";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import { agentsQueryOptions } from "@/hooks/api/use-agents";
import type { Agent, JobRun, PaginatedResponse } from "@/hooks/api/types";
import { runsQueryOptions } from "@/hooks/api/use-runs";
import { FEATURE_FLAGS } from "@/hooks/posthog/flags";
import { useFeatureFlag } from "@/hooks/posthog/use-feature-flag";
import { PlayActionIcon, SparklesIcon } from "@/lib/icons";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/copilot")({
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;

    if (hasProject) {
      await Promise.all([
        context.queryClient
          .ensureQueryData(agentsQueryOptions())
          .catch(() => null),
        context.queryClient
          .ensureQueryData(runsQueryOptions({ limit: 50 }))
          .catch(() => null),
      ]);
    }

    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: CopilotPage,
});

function CopilotPage() {
  const { hasProject, session } = Route.useLoaderData();
  const isAssistantEnabled = useFeatureFlag(FEATURE_FLAGS.AI_ASSISTANT);
  const [prompt, setPrompt] = useState(buildSuggestedPrompts()[0] ?? "");

  const { data: agentsData } = useQuery({
    ...agentsQueryOptions(),
    enabled: hasProject,
  });
  const { data: runsData } = useQuery({
    ...runsQueryOptions({ limit: 50 }),
    enabled: hasProject,
  });

  const agents = (agentsData as Agent[] | undefined) ?? [];
  const recentRuns = (runsData as PaginatedResponse<JobRun> | undefined)?.data ?? [];

  const answer = answerCopilotPrompt(prompt, agents, recentRuns);

  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  if (!isAssistantEnabled) {
    return (
      <Shell>
        <Card>
          <CardHeader>
            <CardTitle>Copilot Preview Disabled</CardTitle>
            <CardDescription>
              The `ai_assistant` feature flag is disabled for this project.
            </CardDescription>
          </CardHeader>
        </Card>
      </Shell>
    );
  }

  const suggestedPrompts = buildSuggestedPrompts();

  return (
    <Shell>
      <div className="grid gap-6 lg:grid-cols-[1.2fr_0.8fr]">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <HugeiconsIcon className="size-5 text-primary" icon={SparklesIcon} />
              Agents Copilot
            </CardTitle>
            <CardDescription>
              Local-only project assistant powered by the agents and runs already loaded
              into the dashboard.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <Textarea
              className="min-h-32"
              onChange={(event) => setPrompt(event.target.value)}
              placeholder="Ask about failures, coverage gaps, or how to evaluate an agent locally..."
              value={prompt}
            />

            <div className="flex flex-wrap gap-2">
              {suggestedPrompts.map((suggestion) => (
                <Button
                  key={suggestion}
                  onClick={() => setPrompt(suggestion)}
                  type="button"
                  variant="outline"
                >
                  {suggestion}
                </Button>
              ))}
            </div>

            <Card className="border-dashed">
              <CardHeader>
                <CardTitle className="text-base">{answer.title}</CardTitle>
                <CardDescription>{answer.summary}</CardDescription>
              </CardHeader>
              <CardContent>
                <ul className="space-y-2 text-sm">
                  {answer.bullets.map((bullet) => (
                    <li key={bullet} className="flex gap-2">
                      <span className="mt-1 size-1.5 rounded-full bg-primary" />
                      <span>{bullet}</span>
                    </li>
                  ))}
                </ul>
              </CardContent>
            </Card>
          </CardContent>
        </Card>

        <div className="grid gap-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Loaded Context</CardTitle>
              <CardDescription>
                Copilot is currently reasoning over dashboard data only.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-2 text-sm">
              <div>{agents.length} agents loaded from the current project.</div>
              <div>{recentRuns.length} recent runs loaded for local analysis.</div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">Recommended Next Step</CardTitle>
              <CardDescription>
                Use the local eval framework before widening the prompt or tool surface.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3 text-sm">
              <div>
                Start with `@strait/agents-sdk` evals, then run the target agent through
                `apps/agents`, and only then expand workflow orchestration or tool access.
              </div>
              <Button
                onClick={() => setPrompt("How should I evaluate a new agent locally?")}
                type="button"
                variant="outline"
              >
                <HugeiconsIcon className="size-4" icon={PlayActionIcon} />
                Generate Eval Guidance
              </Button>
            </CardContent>
          </Card>
        </div>
      </div>
    </Shell>
  );
}
