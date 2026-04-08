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
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import {
  notifySubscribersQueryOptions,
  notifyTopicsQueryOptions,
  useAddNotifyTopicSubscriber,
  useCreateNotifyTopic,
  useRemoveNotifyTopicSubscriber,
} from "@/hooks/api/use-notify";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/notify/topics")({
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await Promise.all([
        context.queryClient.ensureQueryData(notifyTopicsQueryOptions()),
        context.queryClient.ensureQueryData(notifySubscribersQueryOptions()),
      ]);
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: NotifyTopicsPage,
});

function NotifyTopicsPage() {
  const { hasProject, session } = Route.useLoaderData();

  const topicsQuery = useQuery({
    ...notifyTopicsQueryOptions(),
    enabled: hasProject,
  });
  const subscribersQuery = useQuery({
    ...notifySubscribersQueryOptions(),
    enabled: hasProject,
  });

  const createTopic = useCreateNotifyTopic();
  const addSubscriber = useAddNotifyTopicSubscriber();
  const removeSubscriber = useRemoveNotifyTopicSubscriber();

  const [topicKey, setTopicKey] = useState("");
  const [topicName, setTopicName] = useState("");
  const [topicDescription, setTopicDescription] = useState("");

  const [selectedTopicKey, setSelectedTopicKey] = useState("");
  const [selectedSubscriberID, setSelectedSubscriberID] = useState("");

  const topics = topicsQuery.data ?? [];
  const subscribers = subscribersQuery.data ?? [];

  const sortedTopics = useMemo(
    () => [...topics].sort((a, b) => b.created_at.localeCompare(a.created_at)),
    [topics]
  );

  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  const create = async () => {
    if (!(topicKey.trim() && topicName.trim())) {
      toast.error("Topic key and name are required");
      return;
    }

    await toast.promise(
      createTopic.mutateAsync({
        topic_key: topicKey.trim(),
        name: topicName.trim(),
        description: topicDescription.trim() || undefined,
      }),
      {
        loading: "Creating topic...",
        success: "Topic created",
        error: "Failed to create topic",
      }
    );

    setTopicKey("");
    setTopicName("");
    setTopicDescription("");
  };

  const add = async () => {
    if (!(selectedTopicKey && selectedSubscriberID)) {
      toast.error("Select both topic and subscriber");
      return;
    }

    await toast.promise(
      addSubscriber.mutateAsync({
        topicKey: selectedTopicKey,
        subscriber_id: selectedSubscriberID,
      }),
      {
        loading: "Adding subscriber...",
        success: "Subscriber added to topic",
        error: "Failed to add subscriber",
      }
    );
  };

  const remove = async () => {
    if (!(selectedTopicKey && selectedSubscriberID)) {
      toast.error("Select both topic and subscriber");
      return;
    }

    await toast.promise(
      removeSubscriber.mutateAsync({
        topicKey: selectedTopicKey,
        subscriberId: selectedSubscriberID,
      }),
      {
        loading: "Removing subscriber...",
        success: "Subscriber removed from topic",
        error: "Failed to remove subscriber",
      }
    );
  };

  return (
    <Shell>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Create topic</CardTitle>
            <CardDescription>
              Topics group subscribers for fan-out Notify triggers.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid gap-3 md:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="topic-key">Topic key</Label>
                <Input
                  id="topic-key"
                  onChange={(event) => setTopicKey(event.target.value)}
                  placeholder="workflow.approvals"
                  value={topicKey}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="topic-name">Name</Label>
                <Input
                  id="topic-name"
                  onChange={(event) => setTopicName(event.target.value)}
                  placeholder="Workflow approvals"
                  value={topicName}
                />
              </div>
            </div>
            <div className="space-y-1">
              <Label htmlFor="topic-description">Description</Label>
              <Input
                id="topic-description"
                onChange={(event) => setTopicDescription(event.target.value)}
                placeholder="Recipients for approval notifications"
                value={topicDescription}
              />
            </div>
            <Button onClick={create}>Create topic</Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Topic membership</CardTitle>
            <CardDescription>
              Add or remove subscribers from a selected topic.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-1">
              <Label htmlFor="membership-topic">Topic key</Label>
              <Input
                id="membership-topic"
                list="notify-topics"
                onChange={(event) => setSelectedTopicKey(event.target.value)}
                placeholder="workflow.approvals"
                value={selectedTopicKey}
              />
              <datalist id="notify-topics">
                {sortedTopics.map((topic) => (
                  <option key={topic.id} value={topic.topic_key} />
                ))}
              </datalist>
            </div>
            <div className="space-y-1">
              <Label htmlFor="membership-subscriber">Subscriber ID</Label>
              <Input
                id="membership-subscriber"
                list="notify-subscribers"
                onChange={(event) => setSelectedSubscriberID(event.target.value)}
                placeholder="subscriber id"
                value={selectedSubscriberID}
              />
              <datalist id="notify-subscribers">
                {subscribers.map((subscriber) => (
                  <option key={subscriber.id} value={subscriber.id}>
                    {subscriber.external_id}
                  </option>
                ))}
              </datalist>
            </div>
            <div className="flex gap-2">
              <Button onClick={add}>Add subscriber</Button>
              <Button onClick={remove} variant="outline">
                Remove subscriber
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card className="mt-4">
        <CardHeader>
          <CardTitle className="text-sm">Topics</CardTitle>
          <CardDescription>Current topics configured for this project.</CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Topic key</TableHead>
                <TableHead>Name</TableHead>
                <TableHead>Description</TableHead>
                <TableHead>Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedTopics.length === 0 ? (
                <TableRow>
                  <TableCell className="text-muted-foreground" colSpan={4}>
                    No topics yet.
                  </TableCell>
                </TableRow>
              ) : (
                sortedTopics.map((topic) => (
                  <TableRow key={topic.id}>
                    <TableCell>{topic.topic_key}</TableCell>
                    <TableCell>{topic.name}</TableCell>
                    <TableCell>{topic.description || "-"}</TableCell>
                    <TableCell>{new Date(topic.created_at).toLocaleString()}</TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </Shell>
  );
}
