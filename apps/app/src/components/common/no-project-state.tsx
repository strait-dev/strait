import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@strait/ui/components/empty";
import { useState } from "react";
import { RESOURCE_TABLE_EMPTY_CLASS_NAME } from "@/components/tables/resource-table";
import { BriefcaseIcon, PlusIcon } from "@/lib/icons";
import type { AuthUser } from "@/routes/__root";
import CreateProjectDialog from "../project/create-project-dialog";

type Props = {
  user: AuthUser;
};

const NoProjectState = ({ user }: Props) => {
  const [createOpen, setCreateOpen] = useState(false);
  const organizationId = user.defaultOrganizationId;

  return (
    <Empty className={RESOURCE_TABLE_EMPTY_CLASS_NAME}>
      <EmptyHeader>
        <EmptyMedia media="icon" size="lg">
          <HugeiconsIcon
            className="size-6 text-foreground"
            icon={BriefcaseIcon}
          />
        </EmptyMedia>
        <EmptyTitle>No project selected</EmptyTitle>
        <EmptyDescription>
          Create a project to start managing your jobs and workflows.
        </EmptyDescription>
      </EmptyHeader>

      {organizationId ? (
        <EmptyContent>
          <Button onClick={() => setCreateOpen(true)}>
            <HugeiconsIcon className="size-4" icon={PlusIcon} />
            Create project
          </Button>
          <CreateProjectDialog
            onOpenChange={setCreateOpen}
            open={createOpen}
            organizationId={organizationId}
          />
        </EmptyContent>
      ) : (
        <EmptyContent className="text-muted-foreground text-sm">
          Create an organization first, then create a project inside it.
        </EmptyContent>
      )}
    </Empty>
  );
};

export default NoProjectState;
