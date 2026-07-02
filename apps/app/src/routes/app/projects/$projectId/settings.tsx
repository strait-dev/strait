import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Shell } from "@strait/ui/components/shell";
import { createFileRoute } from "@tanstack/react-router";
import DefaultCatchBoundary from "@/components/common/default-catch-boundary";
import NotFound from "@/components/common/not-found";

export const Route = createFileRoute("/app/projects/$projectId/settings")({
  head: () => ({ meta: [{ title: "Project settings · Strait" }] }),
  errorComponent: DefaultCatchBoundary,
  notFoundComponent: () => <NotFound />,
  component: RouteComponent,
});

function RouteComponent() {
  return (
    <Shell>
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Project settings</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-muted-foreground text-sm">
            No configurable project settings are available at launch.
          </p>
        </CardContent>
      </Card>
    </Shell>
  );
}
