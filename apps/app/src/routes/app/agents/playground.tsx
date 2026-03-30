import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Input } from "@strait/ui/components/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { Shell } from "@strait/ui/components/shell";
import { Textarea } from "@strait/ui/components/textarea";
import { useMutation } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { useRef, useState } from "react";

import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import { useAgentStream } from "@/hooks/use-agent-stream";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import type { AppRouteContext } from "@/routes/app/layout";

const MODELS = [
  "gpt-5.4-mini",
  "gpt-5.4",
  "claude-sonnet-4-6",
  "claude-haiku-4-5",
];

const DEFAULT_SYSTEM_PROMPT =
  "You are a helpful support assistant. Classify the user's request and provide a concise answer.";

const runPlayground = createServerFn({ method: "POST" })
  .inputValidator(
    (data: {
      model: string;
      systemPrompt: string;
      maxIterations: number;
      budget: string;
      payload: string;
    }) => data
  )
  .middleware([authMiddleware])
  .handler(({ data }) => {
    return runWithSentryReport(
      apiEffect<{ run_id: string; agent_id: string }>(
        "/v1/agents/playground/run",
        {
          method: "POST",
          body: {
            model: data.model,
            system_prompt: data.systemPrompt,
            max_iterations: data.maxIterations,
            budget: data.budget || undefined,
            payload: data.payload
              ? JSON.parse(data.payload)
              : { prompt: "Hello" },
          },
        }
      )
    );
  });

export const Route = createFileRoute("/app/agents/playground" as any)({
  loader: ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: PlaygroundPage,
});

function getStatusLabel(runId: string | null, connected: boolean): string {
  if (!runId) {
    return "Idle";
  }
  return connected ? "Streaming" : "Waiting";
}

function StreamContent({
  activeRunId,
  chunks,
}: {
  activeRunId: string | null;
  chunks: string[];
}) {
  if (chunks.length > 0) {
    return (
      <>
        {chunks.map((chunk, i) => (
          <span
            key={`pg-${
              // biome-ignore lint/suspicious/noArrayIndexKey: append-only stream
              i
            }`}
          >
            {chunk}
          </span>
        ))}
      </>
    );
  }
  if (activeRunId) {
    return (
      <span className="text-muted-foreground">Waiting for stream data...</span>
    );
  }
  return (
    <div className="space-y-2 text-muted-foreground">
      <p>Configure your agent on the left and click "Run Agent" to start.</p>
      <p className="text-xs">
        The execution stream will appear here in real time.
      </p>
    </div>
  );
}

function PlaygroundPage() {
  const { hasProject, session } = Route.useLoaderData();
  const [model, setModel] = useState(MODELS[0]);
  const [systemPrompt, setSystemPrompt] = useState(DEFAULT_SYSTEM_PROMPT);
  const [maxIterations, setMaxIterations] = useState(5);
  const [budget, setBudget] = useState("");
  const [payload, setPayload] = useState(
    '{\n  "prompt": "My order has not arrived yet"\n}'
  );
  const [activeRunId, setActiveRunId] = useState<string | null>(null);
  const [runHistory, setRunHistory] = useState<
    Array<{ runId: string; agentId: string; startedAt: string }>
  >([]);
  const streamRef = useRef<HTMLDivElement>(null);

  const stream = useAgentStream(activeRunId ?? "", !!activeRunId);

  const mutation = useMutation({
    mutationFn: (data: {
      model: string;
      systemPrompt: string;
      maxIterations: number;
      budget: string;
      payload: string;
    }) => runPlayground({ data }),
    onSuccess: (result) => {
      const res = result as { run_id: string; agent_id: string };
      setActiveRunId(res.run_id);
      setRunHistory((prev) => [
        {
          runId: res.run_id,
          agentId: res.agent_id,
          startedAt: new Date().toISOString(),
        },
        ...prev,
      ]);
    },
  });

  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  return (
    <Shell>
      <div className="flex items-center justify-between pt-4 pb-2">
        <h1 className="font-normal text-xl tracking-tight sm:text-2xl">
          Agent Playground
        </h1>
      </div>

      <div className="grid gap-4 lg:grid-cols-[280px_1fr_260px]">
        {/* Left Panel: Config */}
        <Card className="h-fit">
          <CardHeader className="pb-3">
            <CardTitle className="text-sm">Configuration</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div>
              <label
                className="mb-1 block text-muted-foreground text-xs"
                htmlFor="pg-model"
              >
                Model
              </label>
              <Select
                onValueChange={(v) => {
                  if (v) {
                    setModel(v);
                  }
                }}
                value={model}
              >
                <SelectTrigger id="pg-model">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {MODELS.map((m) => (
                    <SelectItem key={m} value={m}>
                      {m}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div>
              <label
                className="mb-1 block text-muted-foreground text-xs"
                htmlFor="pg-prompt"
              >
                System Prompt
              </label>
              <Textarea
                className="min-h-20 text-xs"
                id="pg-prompt"
                onChange={(e) => setSystemPrompt(e.target.value)}
                value={systemPrompt}
              />
            </div>

            <div className="grid grid-cols-2 gap-2">
              <div>
                <label
                  className="mb-1 block text-muted-foreground text-xs"
                  htmlFor="pg-iter"
                >
                  Max Iterations
                </label>
                <Input
                  id="pg-iter"
                  max={20}
                  min={1}
                  onChange={(e) => setMaxIterations(Number(e.target.value))}
                  type="number"
                  value={maxIterations}
                />
              </div>
              <div>
                <label
                  className="mb-1 block text-muted-foreground text-xs"
                  htmlFor="pg-budget"
                >
                  Budget
                </label>
                <Input
                  id="pg-budget"
                  onChange={(e) => setBudget(e.target.value)}
                  placeholder="$1.00"
                  value={budget}
                />
              </div>
            </div>

            <div>
              <label
                className="mb-1 block text-muted-foreground text-xs"
                htmlFor="pg-payload"
              >
                Payload (JSON)
              </label>
              <Textarea
                className="min-h-16 font-mono text-xs"
                id="pg-payload"
                onChange={(e) => setPayload(e.target.value)}
                value={payload}
              />
            </div>

            <Button
              className="w-full"
              disabled={mutation.isPending}
              onClick={() =>
                mutation.mutate({
                  model,
                  systemPrompt,
                  maxIterations,
                  budget,
                  payload,
                })
              }
              type="button"
            >
              {mutation.isPending ? "Running..." : "Run Agent"}
            </Button>
          </CardContent>
        </Card>

        {/* Center Panel: Execution Stream */}
        <Card className="min-h-[400px]">
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-sm">
              Execution
              {stream.connected && (
                <span className="size-2 rounded-full bg-green-500" />
              )}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div
              className="max-h-[500px] overflow-y-auto whitespace-pre-wrap rounded bg-muted p-3 font-mono text-xs"
              ref={streamRef}
            >
              <StreamContent activeRunId={activeRunId} chunks={stream.chunks} />
            </div>
            {stream.error && (
              <p className="mt-2 text-destructive text-xs">{stream.error}</p>
            )}
          </CardContent>
        </Card>

        {/* Right Panel: Observability */}
        <Card className="h-fit">
          <CardHeader className="pb-3">
            <CardTitle className="text-sm">Observability</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3 text-xs">
            <div className="flex justify-between">
              <span className="text-muted-foreground">Status</span>
              <span>{getStatusLabel(activeRunId, stream.connected)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Chunks</span>
              <span>{stream.chunks.length}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Model</span>
              <span className="font-mono">{model}</span>
            </div>
            {activeRunId && (
              <div className="flex justify-between">
                <span className="text-muted-foreground">Run ID</span>
                <span className="max-w-[140px] truncate font-mono">
                  {activeRunId}
                </span>
              </div>
            )}

            {runHistory.length > 0 && (
              <div className="mt-4 border-t pt-3">
                <p className="mb-2 font-medium text-muted-foreground">
                  History
                </p>
                <div className="space-y-1">
                  {runHistory.slice(0, 5).map((run) => (
                    <div
                      className="flex justify-between text-[11px]"
                      key={run.runId}
                    >
                      <span className="max-w-[120px] truncate font-mono">
                        {run.runId.slice(0, 12)}...
                      </span>
                      <span className="text-muted-foreground">
                        {new Date(run.startedAt).toLocaleTimeString()}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </Shell>
  );
}
