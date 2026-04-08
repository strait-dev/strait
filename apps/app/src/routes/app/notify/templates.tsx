import { HugeiconsIcon } from "@hugeicons/react";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@strait/ui/components/table";
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import type { NotificationTemplate } from "@/hooks/api/types";
import {
  notifyTemplatesQueryOptions,
  useCreateNotificationTemplate,
  useNotifyPreview,
  useUpdateNotificationTemplate,
} from "@/hooks/api/use-notify";
import { SparklesIcon } from "@/lib/icons";
import {
  notifyCursorPageLimit,
  resolveNotifyNextCursor,
} from "@/lib/notify-cursor";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/notify/templates")({
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(
        notifyTemplatesQueryOptions({ limit: notifyCursorPageLimit })
      );
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: NotifyTemplatesPage,
});

const defaultChannels = {
  inbox: {
    title: "{{payload.title}}",
    body: "{{payload.body}}",
  },
  email: {
    subject: "{{payload.subject}}",
    html_body: "<p>{{payload.body}}</p>",
    text_body: "{{payload.body}}",
  },
};

function NotifyTemplatesPage() {
  const { hasProject, session } = Route.useLoaderData();

  const [cursor, setCursor] = useState<string>();
  const [cursorHistory, setCursorHistory] = useState<(string | undefined)[]>(
    []
  );

  const templatesQuery = useQuery({
    ...notifyTemplatesQueryOptions({
      limit: notifyCursorPageLimit,
      cursor,
    }),
    enabled: hasProject,
  });

  const createTemplate = useCreateNotificationTemplate();
  const updateTemplate = useUpdateNotificationTemplate();
  const previewTemplate = useNotifyPreview();

  const [templateKey, setTemplateKey] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [channelsJSON, setChannelsJSON] = useState(
    JSON.stringify(defaultChannels, null, 2)
  );
  const [payloadJSON, setPayloadJSON] = useState(
    JSON.stringify(
      {
        title: "Approval pending",
        body: "A workflow step is waiting for your action.",
        subject: "Approval required",
      },
      null,
      2
    )
  );
  const [previewResult, setPreviewResult] = useState<string>("");
  const [selectedTemplate, setSelectedTemplate] =
    useState<NotificationTemplate | null>(null);

  const pageItems = templatesQuery.data ?? [];

  const sortedTemplates = useMemo(
    () =>
      [...pageItems].sort((a, b) => b.created_at.localeCompare(a.created_at)),
    [pageItems]
  );
  const nextCursor = resolveNotifyNextCursor(
    sortedTemplates,
    notifyCursorPageLimit
  );

  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  const parseJSON = <T,>(raw: string, fallback: T): T => {
    try {
      return JSON.parse(raw) as T;
    } catch {
      return fallback;
    }
  };

  const resetForm = () => {
    setTemplateKey("");
    setName("");
    setDescription("");
    setChannelsJSON(JSON.stringify(defaultChannels, null, 2));
    setSelectedTemplate(null);
  };

  const upsertTemplate = async () => {
    const channels = parseJSON<Record<string, object>>(channelsJSON, {});
    if (!(templateKey.trim() || selectedTemplate)) {
      toast.error("Template key is required");
      return;
    }

    const currentKey = selectedTemplate?.template_key || templateKey.trim();
    const body = {
      template_key: currentKey,
      name: name.trim() || currentKey,
      description: description.trim() || undefined,
      channels,
      status: "published",
      default_locale: "en",
    };

    if (selectedTemplate) {
      await toast.promise(
        updateTemplate.mutateAsync({ ...body, templateKey: currentKey }),
        {
          loading: "Updating template...",
          success: "Template updated",
          error: "Failed to update template",
        }
      );
    } else {
      await toast.promise(createTemplate.mutateAsync(body), {
        loading: "Creating template...",
        success: "Template created",
        error: "Failed to create template",
      });
    }

    resetForm();
  };

  const preview = async () => {
    const key = selectedTemplate?.template_key || templateKey.trim();
    if (!key) {
      toast.error("Choose or create a template first");
      return;
    }

    const payload = parseJSON<Record<string, object>>(payloadJSON, {});

    const result = await toast.promise(
      previewTemplate.mutateAsync({
        template_key: key,
        payload,
      }),
      {
        loading: "Rendering preview...",
        success: "Preview generated",
        error: "Failed to render preview",
      }
    );

    setPreviewResult(JSON.stringify(result, null, 2));
  };

  const goToNextPage = () => {
    if (!nextCursor) {
      return;
    }

    setCursorHistory((history) => [...history, cursor]);
    setCursor(nextCursor);
    setSelectedTemplate(null);
  };

  const goToPreviousPage = () => {
    setCursorHistory((history) => {
      if (history.length === 0) {
        return history;
      }

      const nextHistory = [...history];
      const previousCursor = nextHistory.pop();
      setCursor(previousCursor);
      setSelectedTemplate(null);
      return nextHistory;
    });
  };

  return (
    <Shell>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">
              {selectedTemplate ? "Update template" : "Create template"}
            </CardTitle>
            <CardDescription>
              Manage channel payloads with Handlebars-compatible fields.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid gap-3 md:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="template-key">Template key</Label>
                <Input
                  disabled={!!selectedTemplate}
                  id="template-key"
                  onChange={(event) => setTemplateKey(event.target.value)}
                  placeholder="approval.reminder"
                  value={selectedTemplate?.template_key || templateKey}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="template-name">Name</Label>
                <Input
                  id="template-name"
                  onChange={(event) => setName(event.target.value)}
                  placeholder="Approval reminder"
                  value={name}
                />
              </div>
            </div>

            <div className="space-y-1">
              <Label htmlFor="template-description">Description</Label>
              <Input
                id="template-description"
                onChange={(event) => setDescription(event.target.value)}
                placeholder="Sent when approval is pending"
                value={description}
              />
            </div>

            <div className="space-y-1">
              <Label htmlFor="template-channels">Channels JSON</Label>
              <Textarea
                className="min-h-[220px] font-mono text-xs"
                id="template-channels"
                onChange={(event) => setChannelsJSON(event.target.value)}
                value={channelsJSON}
              />
            </div>

            <div className="flex gap-2">
              <Button onClick={upsertTemplate}>
                {selectedTemplate ? "Update template" : "Create template"}
              </Button>
              {selectedTemplate ? (
                <Button onClick={resetForm} variant="outline">
                  Cancel edit
                </Button>
              ) : null}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Template preview</CardTitle>
            <CardDescription>
              Render channels with sample payload values before triggering
              sends.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-1">
              <Label htmlFor="preview-payload">Payload JSON</Label>
              <Textarea
                className="min-h-[180px] font-mono text-xs"
                id="preview-payload"
                onChange={(event) => setPayloadJSON(event.target.value)}
                value={payloadJSON}
              />
            </div>

            <Button onClick={preview} variant="outline">
              <HugeiconsIcon className="mr-1.5 size-4" icon={SparklesIcon} />
              Render preview
            </Button>

            <Textarea
              className="min-h-[160px] font-mono text-xs"
              readOnly
              value={previewResult}
            />
          </CardContent>
        </Card>
      </div>

      <Card className="mt-4">
        <CardHeader>
          <CardTitle className="text-sm">Templates</CardTitle>
          <CardDescription>
            Click a template row to edit its next version.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Template key</TableHead>
                <TableHead>Name</TableHead>
                <TableHead>Version</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Updated</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedTemplates.length === 0 ? (
                <TableRow>
                  <TableCell className="text-muted-foreground" colSpan={5}>
                    No templates yet.
                  </TableCell>
                </TableRow>
              ) : (
                sortedTemplates.map((template) => (
                  <TableRow
                    className="cursor-pointer"
                    key={template.id}
                    onClick={() => {
                      setSelectedTemplate(template);
                      setTemplateKey(template.template_key);
                      setName(template.name);
                      setDescription(template.description || "");
                      setChannelsJSON(
                        JSON.stringify(template.channels, null, 2)
                      );
                    }}
                  >
                    <TableCell>{template.template_key}</TableCell>
                    <TableCell>{template.name}</TableCell>
                    <TableCell>{template.version}</TableCell>
                    <TableCell>{template.status}</TableCell>
                    <TableCell>
                      {new Date(template.updated_at).toLocaleString()}
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>

          <div className="mt-3 flex items-center justify-between gap-2">
            <p className="text-muted-foreground text-xs">
              Showing up to {notifyCursorPageLimit} templates per page.
            </p>
            <div className="flex gap-2">
              <Button
                disabled={
                  cursorHistory.length === 0 || templatesQuery.isFetching
                }
                onClick={goToPreviousPage}
                variant="outline"
              >
                Previous page
              </Button>
              <Button
                disabled={!nextCursor || templatesQuery.isFetching}
                onClick={goToNextPage}
                variant="outline"
              >
                Next page
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    </Shell>
  );
}
