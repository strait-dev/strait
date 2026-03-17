import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { PlusIcon } from "@/lib/icons";

type RequireOrganizationProps = {
  children: ReactNode;
  hasOrganization: boolean;
  organizationId?: string | null;
  feature?: string;
};

export const RequireOrganization = ({
  children,
  hasOrganization,
  organizationId,
  feature = "this feature",
}: RequireOrganizationProps) => {
  // If user has an organization, render children normally
  if (hasOrganization && organizationId) {
    return <>{children}</>;
  }

  // Otherwise, show a friendly message to create an organization
  return (
    <div className="flex h-[450px] items-center justify-center p-8">
      <Card className="max-w-md">
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-lg bg-muted">
            <HugeiconsIcon
              className="size-6 text-muted-foreground"
              icon={PlusIcon}
            />
          </div>
          <CardTitle>Create an Organization</CardTitle>
        </CardHeader>
        <CardContent className="text-center">
          <p className="mb-6 text-muted-foreground">
            You need to create an organization before you can use {feature}.
          </p>
          <Button
            className="w-full"
            render={<Link preload="intent" to="/app" />}
          >
            <HugeiconsIcon className="size-4" icon={PlusIcon} />
            Create Organization
          </Button>
          <p className="mt-4 text-muted-foreground text-sm">
            Organizations let you manage jobs, workflows, environments, and team
            members.
          </p>
        </CardContent>
      </Card>
    </div>
  );
};
