import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { useState } from "react";
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
    <div className="flex h-[300px] flex-col items-center justify-center gap-4 rounded-lg border border-muted-foreground/10 border-dashed p-8 text-center">
      <div>
        <div className="flex aspect-square h-14 items-center justify-center rounded-lg bg-muted">
          <HugeiconsIcon
            className="size-6 text-muted-foreground"
            icon={BriefcaseIcon}
          />
        </div>
      </div>

      <div className="flex max-w-xs flex-col items-center gap-2 text-center">
        <h2 className="text-balance font-normal text-lg text-secondary-foreground tracking-tight">
          No project selected
        </h2>
        <p className="text-pretty text-muted-foreground text-sm">
          Create a project to start managing your jobs and workflows.
        </p>
      </div>

      {organizationId ? (
        <>
          <Button className="mt-2" onClick={() => setCreateOpen(true)}>
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
    </div>
  );
};

export default NoProjectState;
