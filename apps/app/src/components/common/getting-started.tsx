import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { useState } from "react";
import { CheckCircleIcon, PlusIcon } from "@/lib/icons";
import type { AuthUser } from "@/routes/__root";
import CreateProjectDialog from "../project/create-project-dialog";

type Props = {
  user: AuthUser;
};

export const GettingStarted = ({ user }: Props) => {
  const [createOpen, setCreateOpen] = useState(false);
  const organizationId = user.defaultOrganizationId;
  const hasProject = !!user.activeProjectId;

  const steps = [
    {
      title: "Create a project",
      description: "Projects organize your jobs and workflows.",
      done: hasProject,
      action: organizationId ? (
        <Button onClick={() => setCreateOpen(true)} size="sm">
          <HugeiconsIcon className="size-4" icon={PlusIcon} />
          Create project
        </Button>
      ) : null,
    },
    {
      title: "Install the SDK",
      description: "Add the Strait SDK to your application.",
      done: false,
      code: "npm install @strait/sdk",
    },
    {
      title: "Deploy your first job",
      description: "Register and deploy a job to start processing work.",
      done: false,
      code: `import { Strait } from "@strait/sdk";

const strait = new Strait({ apiKey: "your-api-key" });

strait.job("hello-world", async (payload) => {
  console.log("Hello from Strait!", payload);
  return { success: true };
});`,
    },
  ];

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="font-normal text-2xl tracking-tight">Get started</h1>
        <p className="mt-1 text-muted-foreground">
          Follow these steps to set up your first job.
        </p>
      </div>

      <div className="space-y-4">
        {steps.map((step, i) => (
          <Card key={step.title}>
            <CardHeader className="flex-row items-center gap-3 space-y-0 pb-2">
              <div
                className={`flex size-8 shrink-0 items-center justify-center rounded-full border ${
                  step.done
                    ? "border-chart-1 bg-chart-1/10 text-chart-1"
                    : "border-border text-muted-foreground"
                }`}
              >
                {step.done ? (
                  <HugeiconsIcon className="size-4" icon={CheckCircleIcon} />
                ) : (
                  <span className="font-medium text-sm">{i + 1}</span>
                )}
              </div>
              <CardTitle className="text-base">{step.title}</CardTitle>
            </CardHeader>
            <CardContent className="pl-14">
              <p className="text-muted-foreground text-sm">
                {step.description}
              </p>
              {step.code ? (
                <pre className="mt-3 overflow-x-auto rounded-lg bg-muted p-3 font-mono text-sm">
                  <code>{step.code}</code>
                </pre>
              ) : null}
              {step.action && !step.done ? (
                <div className="mt-3">{step.action}</div>
              ) : null}
            </CardContent>
          </Card>
        ))}
      </div>

      {organizationId ? (
        <CreateProjectDialog
          onOpenChange={setCreateOpen}
          open={createOpen}
          organizationId={organizationId}
        />
      ) : null}
    </div>
  );
};
