import { Badge } from "@strait/ui/components/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemHeader,
  ItemTitle,
} from "@strait/ui/components/item";
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
          <ItemGroup className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {regions.map((region) => {
              const isAvailable =
                region.availability?.[settings.plan_tier] === true;
              const isSelected = settings.default_region === region.code;
              const isDisabled = !isAvailable || updateSettings.isPending;
              const updateRegion = () => {
                if (isDisabled) {
                  return;
                }
                const promise = updateSettings.mutateAsync({
                  projectId,
                  default_region: region.code,
                });
                toast.promise(promise, {
                  loading: "Updating region...",
                  success: "Default region updated!",
                  error: "Failed to update region",
                });
              };

              return (
                <Item
                  aria-disabled={isDisabled}
                  aria-pressed={isSelected}
                  className={`items-start ${isAvailable ? "" : "cursor-not-allowed opacity-50"}`}
                  key={region.code}
                  onClick={updateRegion}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" || event.key === " ") {
                      event.preventDefault();
                      updateRegion();
                    }
                  }}
                  role="button"
                  tabIndex={isDisabled ? -1 : 0}
                  variant="outline"
                >
                  <ItemHeader>
                    <ItemTitle>{region.label}</ItemTitle>
                    <ItemActions>
                      {isSelected && <Badge variant="default">Active</Badge>}
                    </ItemActions>
                  </ItemHeader>
                  <ItemContent>
                    <ItemDescription>
                      {region.city}, {region.country}
                    </ItemDescription>
                    {!isAvailable && (
                      <ItemDescription>Upgrade to unlock</ItemDescription>
                    )}
                  </ItemContent>
                </Item>
              );
            })}
          </ItemGroup>
        </CardContent>
      </Card>
    </div>
  );
};

export default ProjectSettings;
