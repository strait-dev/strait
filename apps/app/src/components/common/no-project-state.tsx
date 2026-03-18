import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { useState } from "react";
import { BriefcaseIcon, PlusIcon } from "@/lib/icons";
import type { AuthUser } from "@/routes/__root";
import CreateProjectDialog from "../project/create-project-dialog";

type Props = {
  user: AuthUser;
};

export const NoProjectState = ({ user }: Props) => {
  const [createOpen, setCreateOpen] = useState(false);
  const organizationId = user.defaultOrganizationId;

  return (
    <div className="flex h-[450px] items-center justify-center p-8">
      <Card className="max-w-md">
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-lg bg-muted">
            <HugeiconsIcon
              className="size-6 text-muted-foreground"
              icon={BriefcaseIcon}
            />
          </div>
          <CardTitle>No project selected</CardTitle>
        </CardHeader>
        <CardContent className="text-center">
          <p className="mb-6 text-muted-foreground">
            Create a project to start managing your jobs and workflows.
          </p>
          {organizationId ? (
            <>
              <Button className="w-full" onClick={() => setCreateOpen(true)}>
                <HugeiconsIcon className="size-4" icon={PlusIcon} />
                Create project
              </Button>
              <CreateProjectDialog
                onOpenChange={setCreateOpen}
                open={createOpen}
                organizationId={organizationId}
              />
            </>
          ) : (
            <p className="text-muted-foreground text-sm">
              Create an organization first, then create a project inside it.
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  );
};
