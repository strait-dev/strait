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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { Shell } from "@strait/ui/components/shell";
import { Switch } from "@strait/ui/components/switch";
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import type { NotifyDeliveryChannel } from "@/hooks/api/types";
import {
  notifySubscribersQueryOptions,
  notifyTemplatesQueryOptions,
  notifyTopicsQueryOptions,
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
        context.queryClient.ensureQueryData(notifyTopicsQueryOptions()),
      ]);
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: NotifyComposePage,
});

const recipientTypeOptions = ["subscriber", "topic"] as const;
const channelOptions: readonly NotifyDeliveryChannel[] = [
  "inbox",
  "email",
] as const;

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
  const topicsQuery = useQuery({
    ...notifyTopicsQueryOptions(),
    enabled: hasProject,
  });

  const trigger = useNotifyTrigger();
  const testSend = useNotifyTest();
  const preview = useNotifyPreview();

  const [recipientType, setRecipientType] =
    useState<(typeof recipientTypeOptions)[number]>("subscriber");
  const [recipientID, setRecipientID] = useState("");
  const [recipientKey, setRecipientKey] = useState("");
  const [templateKey, setTemplateKey] = useState("");
  const [selectedChannels, setSelectedChannels] = useState<
    NotifyDeliveryChannel[]
  >(["inbox", "email"]);
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
  const [formErrors, setFormErrors] = useState<{
    templateKey?: string;
    recipient?: string;
    channels?: string;
    payload?: string;
  }>({});

  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  const templates = templatesQuery.data ?? [];
  const subscribers = subscribersQuery.data ?? [];
  const topics = topicsQuery.data ?? [];

  const parsePayload = () => {
    try {
      return JSON.parse(payloadJSON) as Record<string, object>;
    } catch {
      return null;
    }
  };

  const parseChannels = (): NotifyDeliveryChannel[] => [...selectedChannels];

  const validateRecipient = (
    nextErrors: Partial<typeof formErrors>
  ):
    | { type: "subscriber"; id: string }
    | { type: "topic"; key: string }
    | null => {
    if (recipientType === "topic") {
      if (!recipientKey.trim()) {
        nextErrors.recipient = "Topic is required";
        return null;
      }
      return { type: "topic", key: recipientKey.trim() };
    }

    if (!recipientID.trim()) {
      nextErrors.recipient = "Subscriber ID is required";
      return null;
    }

    return { type: "subscriber", id: recipientID.trim() };
  };

  const validateComposeInput = () => {
    const nextErrors: Partial<typeof formErrors> = {};

    const payload = parsePayload();
    if (!payload) {
      nextErrors.payload = "Payload must be valid JSON";
    }

    if (!templateKey.trim()) {
      nextErrors.templateKey = "Template key is required";
    }

    const channels = parseChannels();
    if (channels.length === 0) {
      nextErrors.channels = "Select at least one channel";
    }

    const recipient = validateRecipient(nextErrors);

    setFormErrors(nextErrors);

    const hasValidCoreInput =
      !!payload && !!recipient && !!templateKey.trim() && channels.length > 0;

    if (!hasValidCoreInput) {
      toast.error("Fix form errors before continuing");
      return null;
    }

    return {
      payload,
      channels,
      recipient,
      templateKey: templateKey.trim(),
    };
  };

  const runPreview = async () => {
    const input = validateComposeInput();
    if (!input) {
      return;
    }

    const result = await toast.promise(
      preview.mutateAsync({
        template_key: input.templateKey,
        payload: input.payload,
        channels: input.channels,
        subscriber_id:
          input.recipient.type === "subscriber"
            ? input.recipient.id
            : undefined,
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
    const input = validateComposeInput();
    if (!input) {
      return;
    }

    const body = {
      to: input.recipient,
      template_key: input.templateKey,
      payload: input.payload,
      channels: input.channels,
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

  const isWorking =
    preview.isPending || testSend.isPending || trigger.isPending;

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
                <Select
                  onValueChange={(value) => {
                    setRecipientType(
                      value as (typeof recipientTypeOptions)[number]
                    );
                    setRecipientID("");
                    setRecipientKey("");
                    setFormErrors((errors) => ({
                      ...errors,
                      recipient: undefined,
                    }));
                  }}
                  value={recipientType}
                >
                  <SelectTrigger id="recipient-type">
                    <SelectValue placeholder="Choose recipient type" />
                  </SelectTrigger>
                  <SelectContent>
                    {recipientTypeOptions.map((option) => (
                      <SelectItem key={option} value={option}>
                        {option}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label htmlFor="template-key">Template key</Label>
                <Input
                  id="template-key"
                  list="notify-templates"
                  onChange={(event) => {
                    setTemplateKey(event.target.value);
                    setFormErrors((errors) => ({
                      ...errors,
                      templateKey: undefined,
                    }));
                  }}
                  value={templateKey}
                />
                {formErrors.templateKey ? (
                  <p className="text-destructive text-xs">
                    {formErrors.templateKey}
                  </p>
                ) : null}
                <datalist id="notify-templates">
                  {templates.map((template) => (
                    <option key={template.id} value={template.template_key} />
                  ))}
                </datalist>
              </div>
              {recipientType === "topic" ? (
                <div className="space-y-1 md:col-span-2">
                  <Label htmlFor="recipient-key">Topic key</Label>
                  <Select
                    onValueChange={(value) => {
                      setRecipientKey(value ?? "");
                      setFormErrors((errors) => ({
                        ...errors,
                        recipient: undefined,
                      }));
                    }}
                    value={recipientKey || undefined}
                  >
                    <SelectTrigger id="recipient-key">
                      <SelectValue placeholder="Choose topic" />
                    </SelectTrigger>
                    <SelectContent>
                      {topics.map((topic) => (
                        <SelectItem key={topic.id} value={topic.topic_key}>
                          {topic.topic_key}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              ) : (
                <div className="space-y-1 md:col-span-2">
                  <Label htmlFor="recipient-id">Subscriber ID</Label>
                  <Input
                    id="recipient-id"
                    list="notify-subscribers-compose"
                    onChange={(event) => {
                      setRecipientID(event.target.value);
                      setFormErrors((errors) => ({
                        ...errors,
                        recipient: undefined,
                      }));
                    }}
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
              {formErrors.recipient ? (
                <p className="text-destructive text-xs md:col-span-2">
                  {formErrors.recipient}
                </p>
              ) : null}

              <div className="space-y-2 md:col-span-2">
                <Label>Channels</Label>
                <div className="grid gap-2 md:grid-cols-2">
                  {channelOptions.map((channel) => (
                    <div
                      className="flex items-center justify-between rounded-md border p-3"
                      key={channel}
                    >
                      <div>
                        <p className="font-medium text-sm">{channel}</p>
                      </div>
                      <Switch
                        checked={selectedChannels.includes(channel)}
                        onCheckedChange={(checked) => {
                          setSelectedChannels((current) => {
                            if (checked) {
                              return Array.from(new Set([...current, channel]));
                            }
                            return current.filter((value) => value !== channel);
                          });
                          setFormErrors((errors) => ({
                            ...errors,
                            channels: undefined,
                          }));
                        }}
                      />
                    </div>
                  ))}
                </div>
                {formErrors.channels ? (
                  <p className="text-destructive text-xs">
                    {formErrors.channels}
                  </p>
                ) : null}
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
                onChange={(event) => {
                  setPayloadJSON(event.target.value);
                  setFormErrors((errors) => ({
                    ...errors,
                    payload: undefined,
                  }));
                }}
                value={payloadJSON}
              />
              {formErrors.payload ? (
                <p className="text-destructive text-xs">{formErrors.payload}</p>
              ) : null}
            </div>

            <div className="flex flex-wrap gap-2">
              <Button
                disabled={isWorking}
                onClick={runPreview}
                variant="outline"
              >
                Preview
              </Button>
              <Button
                disabled={isWorking}
                onClick={() => runTrigger("test")}
                variant="secondary"
              >
                Test send
              </Button>
              <Button
                disabled={isWorking}
                onClick={() => runTrigger("trigger")}
              >
                Trigger
              </Button>
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
