import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import { useState } from "react";
import { ArrowUpRightIcon, CheckCircleIcon, PlusIcon } from "@/lib/icons";
import type { AuthUser } from "@/routes/__root";
import CreateProjectDialog from "../project/create-project-dialog";

type Props = {
  user: AuthUser;
};

const SDK_TABS = [
  {
    value: "typescript",
    label: "TypeScript",
    install: "npm install @strait/sdk",
  },
  { value: "python", label: "Python", install: "pip install strait-sdk" },
  {
    value: "go",
    label: "Go",
    install: "go get github.com/straitdev/strait-go",
  },
  { value: "rust", label: "Rust", install: "cargo add strait-sdk" },
  {
    value: "cli",
    label: "CLI",
    install: "cd apps/strait && go install ./cmd/strait",
  },
] as const;

const SDK_TABS_WITHOUT_CLI = SDK_TABS.filter((t) => t.value !== "cli");

const CODE_EXAMPLES: Record<string, string> = {
  typescript: `import { Strait } from "@strait/sdk";

const strait = new Strait({ apiKey: "your-api-key" });

strait.job("hello-world", async (payload) => {
  console.log("Hello from Strait!", payload);
  return { success: true };
});`,
  python: `from strait_sdk import Strait

strait = Strait(api_key="your-api-key")

@strait.job("hello-world")
async def hello_world(payload):
    print("Hello from Strait!", payload)
    return {"success": True}`,
  go: `package main

import "github.com/straitdev/strait-go"

func main() {
    s := strait.New(strait.WithAPIKey("your-api-key"))

    s.Job("hello-world", func(payload any) (any, error) {
        fmt.Println("Hello from Strait!", payload)
        return map[string]bool{"success": true}, nil
    })
}`,
  rust: `use strait_sdk::Strait;

#[tokio::main]
async fn main() {
    let strait = Strait::new("your-api-key");

    strait.job("hello-world", |payload| async move {
        println!("Hello from Strait! {:?}", payload);
        Ok(serde_json::json!({"success": true}))
    });
}`,
};

export const GettingStarted = ({ user }: Props) => {
  const [createOpen, setCreateOpen] = useState(false);
  const [activeTab, setActiveTab] = useState("typescript");
  const organizationId = user.defaultOrganizationId;
  const hasProject = !!user.activeProjectId;

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div>
        <h1 className="text-balance font-normal text-2xl tracking-tight">
          Get started
        </h1>
        <p className="mt-1 text-muted-foreground">
          Follow these steps to set up your first job.
        </p>
      </div>

      <div className="space-y-4">
        {/* Step 1: Create a project */}
        <Card>
          <CardHeader className="flex-row items-center gap-3 space-y-0 pb-2">
            <div
              className={`flex size-8 shrink-0 items-center justify-center rounded-full border ${
                hasProject
                  ? "border-chart-1 bg-chart-1/10 text-chart-1"
                  : "border-border text-muted-foreground"
              }`}
            >
              {hasProject ? (
                <HugeiconsIcon className="size-4" icon={CheckCircleIcon} />
              ) : (
                <span className="font-medium text-sm">1</span>
              )}
            </div>
            <CardTitle className="text-base">Create a project</CardTitle>
          </CardHeader>
          <CardContent className="pl-6 sm:pl-14">
            <p className="text-muted-foreground text-sm">
              Projects organize your jobs and workflows.
            </p>
            {organizationId && !hasProject ? (
              <div className="mt-3">
                <Button onClick={() => setCreateOpen(true)} size="sm">
                  <HugeiconsIcon className="size-4" icon={PlusIcon} />
                  Create project
                </Button>
              </div>
            ) : null}
          </CardContent>
        </Card>

        {/* Step 2: Install the SDK */}
        <Card>
          <CardHeader className="flex-row items-center gap-3 space-y-0 pb-2">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-full border border-border text-muted-foreground">
              <span className="font-medium text-sm">2</span>
            </div>
            <CardTitle className="text-base">Install the SDK</CardTitle>
          </CardHeader>
          <CardContent className="pl-6 sm:pl-14">
            <p className="text-muted-foreground text-sm">
              Add the Strait SDK to your application.
            </p>
            <Tabs
              className="mt-3"
              onValueChange={setActiveTab}
              value={activeTab}
            >
              <TabsList>
                {SDK_TABS.map((tab) => (
                  <TabsTrigger key={tab.value} value={tab.value}>
                    {tab.label}
                  </TabsTrigger>
                ))}
              </TabsList>
              {SDK_TABS.map((tab) => (
                <TabsContent key={tab.value} value={tab.value}>
                  <pre className="overflow-x-auto rounded-lg bg-muted p-3 font-mono text-sm">
                    <code>{tab.install}</code>
                  </pre>
                </TabsContent>
              ))}
            </Tabs>
          </CardContent>
        </Card>

        {/* Step 3: Deploy your first job */}
        <Card>
          <CardHeader className="flex-row items-center gap-3 space-y-0 pb-2">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-full border border-border text-muted-foreground">
              <span className="font-medium text-sm">3</span>
            </div>
            <CardTitle className="text-base">Deploy your first job</CardTitle>
          </CardHeader>
          <CardContent className="pl-6 sm:pl-14">
            <p className="text-muted-foreground text-sm">
              Register and deploy a job to start processing work.
            </p>
            <Tabs
              className="mt-3"
              onValueChange={setActiveTab}
              value={activeTab}
            >
              <TabsList>
                {SDK_TABS_WITHOUT_CLI.map((tab) => (
                  <TabsTrigger key={tab.value} value={tab.value}>
                    {tab.label}
                  </TabsTrigger>
                ))}
              </TabsList>
              {SDK_TABS_WITHOUT_CLI.map((tab) => (
                <TabsContent key={tab.value} value={tab.value}>
                  <pre className="overflow-x-auto rounded-lg bg-muted p-3 font-mono text-sm">
                    <code>{CODE_EXAMPLES[tab.value]}</code>
                  </pre>
                </TabsContent>
              ))}
            </Tabs>
          </CardContent>
        </Card>
      </div>

      {/* Docs link */}
      <div className="flex justify-center pb-4">
        <a
          href="https://docs.strait.dev"
          rel="noopener noreferrer"
          target="_blank"
        >
          <Button variant="outline">
            Read the full documentation
            <HugeiconsIcon className="size-4" icon={ArrowUpRightIcon} />
          </Button>
        </a>
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
