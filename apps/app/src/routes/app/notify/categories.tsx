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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
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
  notifyCategoriesQueryOptions,
  useCreateNotificationCategory,
} from "@/hooks/api/use-notify";
import { isNotifyScopedKey } from "@/lib/notify-form";
import type { AppRouteContext } from "@/routes/app/layout";

const notifyCategoryTypeOptions = [
  "product",
  "transactional",
  "critical",
] as const;

export const Route = createFileRoute("/app/notify/categories")({
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(notifyCategoriesQueryOptions());
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: NotifyCategoriesPage,
});

function NotifyCategoriesPage() {
  const { hasProject, session } = Route.useLoaderData();

  const categoriesQuery = useQuery({
    ...notifyCategoriesQueryOptions(),
    enabled: hasProject,
  });
  const createCategory = useCreateNotificationCategory();

  const [categoryKey, setCategoryKey] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [type, setType] =
    useState<(typeof notifyCategoryTypeOptions)[number]>("product");

  const categories = categoriesQuery.data ?? [];

  const sortedCategories = useMemo(
    () =>
      [...categories].sort((a, b) =>
        a.category_key.localeCompare(b.category_key)
      ),
    [categories]
  );

  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  const create = async () => {
    if (!(categoryKey.trim() && name.trim())) {
      toast.error("Category key and name are required");
      return;
    }
    if (!isNotifyScopedKey(categoryKey)) {
      toast.error(
        "Category key can only include letters, numbers, dots, dashes, and underscores"
      );
      return;
    }

    await toast.promise(
      createCategory.mutateAsync({
        category_key: categoryKey.trim(),
        name: name.trim(),
        description: description.trim() || undefined,
        type,
      }),
      {
        loading: "Creating category...",
        success: "Category created",
        error: "Failed to create category",
      }
    );

    setCategoryKey("");
    setName("");
    setDescription("");
    setType("product");
  };

  return (
    <Shell>
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Create category</CardTitle>
          <CardDescription>
            Categories drive digest and preference semantics for templates.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid gap-3 md:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="category-key">Category key</Label>
              <Input
                id="category-key"
                onChange={(event) => setCategoryKey(event.target.value)}
                placeholder="approvals"
                value={categoryKey}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="category-name">Name</Label>
              <Input
                id="category-name"
                onChange={(event) => setName(event.target.value)}
                placeholder="Approvals"
                value={name}
              />
            </div>
            <div className="space-y-1 md:col-span-2">
              <Label htmlFor="category-description">Description</Label>
              <Input
                id="category-description"
                onChange={(event) => setDescription(event.target.value)}
                placeholder="Notifications for approval workflows"
                value={description}
              />
            </div>
            <div className="space-y-1 md:col-span-2">
              <Label htmlFor="category-type">Type</Label>
              <Select
                onValueChange={(value) =>
                  setType(value as (typeof notifyCategoryTypeOptions)[number])
                }
                value={type}
              >
                <SelectTrigger id="category-type">
                  <SelectValue placeholder="Choose category type" />
                </SelectTrigger>
                <SelectContent>
                  {notifyCategoryTypeOptions.map((option) => (
                    <SelectItem key={option} value={option}>
                      {option}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <Button disabled={createCategory.isPending} onClick={create}>
            Create category
          </Button>
        </CardContent>
      </Card>

      <Card className="mt-4">
        <CardHeader>
          <CardTitle className="text-sm">Categories</CardTitle>
          <CardDescription>
            Current notify categories configured in this project.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Category key</TableHead>
                <TableHead>Name</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Description</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedCategories.length === 0 ? (
                <TableRow>
                  <TableCell className="text-muted-foreground" colSpan={4}>
                    No categories yet.
                  </TableCell>
                </TableRow>
              ) : (
                sortedCategories.map((category) => (
                  <TableRow key={category.id}>
                    <TableCell>{category.category_key}</TableCell>
                    <TableCell>{category.name}</TableCell>
                    <TableCell>{category.type}</TableCell>
                    <TableCell>{category.description || "-"}</TableCell>
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
