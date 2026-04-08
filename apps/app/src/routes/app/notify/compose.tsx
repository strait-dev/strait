import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Input } from "@strait/ui/components/input";
import { Label } from "@strait/ui/components/label";
import { Shell } from "@strait/ui/components/shell";
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import {
  notifySubscribersQueryOptions,
  notifyTemplatesQueryOptions,
  useNotifyPreview,
  useNotifyTest,
  useNotifyTrigger,
} from "@/hooks/api/use-notify";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/notify/compose")({
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await Promise.all([
        context.queryClient.ensureQueryData(notifyTemplatesQueryOptions()),
        context.queryClient.ensureQueryData(notifySubscribersQueryOptions()),
      ]);
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: NotifyComposePage,
});

function NotifyComposePage() {
  const { hasProject, session } = Route.useLoaderData();

  const templatesQuery = useQuery({
    ...notifyTemplatesQueryOptions(),
    enabled: hasProject,
  });
  const subscribersQuery = useQuery({
    ...notifySubscribersQueryOptions(),
    enabled: hasProject,
  });

  const trigger = useNotifyTrigger();
  const testSend = useNotifyTest();
  const preview = useNotifyPreview();

  const [recipientType, setRecipientType] = useState("subscriber");
  const [recipientID, setRecipientID] = useState("");
  const [recipientKey, setRecipientKey] = useState("");
  const [templateKey, setTemplateKey] = useState("");
  const [channelsCSV, setChannelsCSV] = useState("inbox,email");
  const [categoryKey, setCategoryKey] = useState("");
  const [payloadJSON, setPayloadJSON] = useState(
    JSON.stringify(
      {
        title: "Workflow approval needed",
        body: "A step is waiting for your review.",
        subject: "Approval required",
      },
      null,
      2
    )
  );

  const [previewResult, setPreviewResult] = useState("");
  const [triggerResult, setTriggerResult] = useState("");

  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  const templates = templatesQuery.data ?? [];
  const subscribers = subscribersQuery.data ?? [];

  const parsePayload = () => {
    try {
      return JSON.parse(payloadJSON) as Record<string, object>;
    } catch {
      return null;
    }
  };

  const parseChannels = () =>
    channelsCSV
      .split(",")
      .map((value) => value.trim())
      .filter(Boolean);

  const recipient = () => {
    if (recipientType === "topic") {
      return { type: "topic" as const, key: recipientKey.trim() };
    }
    return { type: "subscriber" as const, id: recipientID.trim() };
  };

  const runPreview = async () => {
    const payload = parsePayload();
    if (!payload) {
      toast.error("Payload must be valid JSON");
      return;
    }
    if (!templateKey.trim()) {
      toast.error("Template key is required");
      return;
    }

    const result = await toast.promise(
      preview.mutateAsync({
        template_key: templateKey.trim(),
        payload,
        channels: parseChannels(),
        subscriber_id:
          recipientType === "subscriber" ? recipientID.trim() : undefined,
        category_key: categoryKey.trim() || undefined,
      }),
      {
        loading: "Rendering preview...",
        success: "Preview rendered",
        error: "Failed to render preview",
      }
    );

    setPreviewResult(JSON.stringify(result, null, 2));
  };

  const runTrigger = async (mode: "trigger" | "test") => {
    const payload = parsePayload();
    if (!payload) {
      toast.error("Payload must be valid JSON");
      return;
    }
    if (!templateKey.trim()) {
      toast.error("Template key is required");
      return;
    }

    const body = {
      to: recipient(),
      template_key: templateKey.trim(),
      payload,
      channels: parseChannels(),
      category_key: categoryKey.trim() || undefined,
    };

    const promise =
      mode === "test" ? testSend.mutateAsync(body) : trigger.mutateAsync(body);

    const result = await toast.promise(promise, {
      loading:
        mode === "test" ? "Sending test notify..." : "Triggering notify...",
      success: mode === "test" ? "Test notify sent" : "Notify triggered",
      error: mode === "test" ? "Test notify failed" : "Notify trigger failed",
    });

    setTriggerResult(JSON.stringify(result, null, 2));
  };

  return (
    <Shell>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Compose notify</CardTitle>
            <CardDescription>
              Trigger or test send a notify payload through the standard API
              pipeline.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid gap-3 md:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="recipient-type">Recipient type</Label>
                <Input
                  id="recipient-type"
                  onChange={(event) => setRecipientType(event.target.value)}
                  value={recipientType}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="template-key">Template key</Label>
                <Input
                  id="template-key"
                  list="notify-templates"
                  onChange={(event) => setTemplateKey(event.target.value)}
                  value={templateKey}
                />
                <datalist id="notify-templates">
                  {templates.map((template) => (
                    <option key={template.id} value={template.template_key} />
                  ))}
                </datalist>
              </div>
              {recipientType === "topic" ? (
                <div className="space-y-1 md:col-span-2">
                  <Label htmlFor="recipient-key">Topic key</Label>
                  <Input
                    id="recipient-key"
                    onChange={(event) => setRecipientKey(event.target.value)}
                    value={recipientKey}
                  />
                </div>
              ) : (
                <div className="space-y-1 md:col-span-2">
                  <Label htmlFor="recipient-id">Subscriber ID</Label>
                  <Input
                    id="recipient-id"
                    list="notify-subscribers-compose"
                    onChange={(event) => setRecipientID(event.target.value)}
                    value={recipientID}
                  />
                  <datalist id="notify-subscribers-compose">
                    {subscribers.map((subscriber) => (
                      <option key={subscriber.id} value={subscriber.id}>
                        {subscriber.external_id}
                      </option>
                    ))}
                  </datalist>
                </div>
              )}

              <div className="space-y-1 md:col-span-2">
                <Label htmlFor="channels">Channels CSV</Label>
                <Input
                  id="channels"
                  onChange={(event) => setChannelsCSV(event.target.value)}
                  value={channelsCSV}
                />
              </div>

              <div className="space-y-1 md:col-span-2">
                <Label htmlFor="category-key">Category key (optional)</Label>
                <Input
                  id="category-key"
                  onChange={(event) => setCategoryKey(event.target.value)}
                  value={categoryKey}
                />
              </div>
            </div>

            <div className="space-y-1">
              <Label htmlFor="payload-json">Payload JSON</Label>
              <Textarea
                className="min-h-[220px] font-mono text-xs"
                id="payload-json"
                onChange={(event) => setPayloadJSON(event.target.value)}
                value={payloadJSON}
              />
            </div>

            <div className="flex flex-wrap gap-2">
              <Button onClick={runPreview} variant="outline">
                Preview
              </Button>
              <Button onClick={() => runTrigger("test")} variant="secondary">
                Test send
              </Button>
              <Button onClick={() => runTrigger("trigger")}>Trigger</Button>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Output</CardTitle>
            <CardDescription>
              Preview output and trigger API responses are shown below.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-1">
              <Label htmlFor="preview-output">Preview</Label>
              <Textarea
                className="min-h-[180px] font-mono text-xs"
                id="preview-output"
                readOnly
                value={previewResult}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="trigger-output">Trigger/Test response</Label>
              <Textarea
                className="min-h-[180px] font-mono text-xs"
                id="trigger-output"
                readOnly
                value={triggerResult}
              />
            </div>
          </CardContent>
        </Card>
      </div>
    </Shell>
  );
}
