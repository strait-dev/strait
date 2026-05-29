import { Badge } from "@strait/ui/components/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { toast } from "@strait/ui/components/toast";
import { useSuspenseQuery } from "@tanstack/react-query";
import {
  projectSettingsQueryOptions,
  regionsQueryOptions,
  useUpdateProjectSettings,
} from "@/hooks/api/use-regions";

type Props = {
  projectId: string;
};

const ProjectSettings = ({ projectId }: Props) => {
  const { data: regionsResponse } = useSuspenseQuery(regionsQueryOptions());
  const { data: settings } = useSuspenseQuery(
    projectSettingsQueryOptions(projectId)
  );
  const updateSettings = useUpdateProjectSettings();

  const regions = regionsResponse?.regions ?? [];

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-balance font-normal text-foreground text-lg tracking-tight">
          Project Settings
        </h2>
        <p className="text-muted-foreground text-sm">
          Configure defaults for this project.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Default Region</CardTitle>
          <CardDescription>
            Select the default region for new jobs in this project.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {regions.map((region) => {
              const isAvailable =
                region.availability?.[settings.plan_tier] === true;
              const isSelected = settings.default_region === region.code;

              let borderClass = "cursor-not-allowed border-border opacity-50";
              if (isSelected) {
                borderClass = "border-primary bg-primary/5";
              } else if (isAvailable) {
                borderClass =
                  "border-border hover:border-primary/50 hover:bg-muted/50";
              }

              return (
                <button
                  className={`relative flex flex-col rounded-lg border p-4 text-left transition-colors ${borderClass}`}
                  disabled={!isAvailable || updateSettings.isPending}
                  key={region.code}
                  onClick={() => {
                    const promise = updateSettings.mutateAsync({
                      projectId,
                      default_region: region.code,
                    });
                    toast.promise(promise, {
                      loading: "Updating region...",
                      success: "Default region updated!",
                      error: "Failed to update region",
                    });
                  }}
                  type="button"
                >
                  <div className="flex items-center justify-between">
                    <span className="font-medium text-sm">{region.label}</span>
                    {isSelected && <Badge variant="default">Active</Badge>}
                  </div>
                  <span className="mt-1 text-muted-foreground text-xs">
                    {region.city}, {region.country}
                  </span>
                  {!isAvailable && (
                    <span className="mt-2 text-muted-foreground text-xs">
                      Upgrade to unlock
                    </span>
                  )}
                </button>
              );
            })}
          </div>
        </CardContent>
      </Card>
    </div>
  );
};

export default ProjectSettings;
