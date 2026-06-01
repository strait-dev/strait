import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
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
  ItemMedia,
  ItemTitle,
} from "@strait/ui/components/item";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import {
  accountsQueryOptions,
  useUnlinkAccount,
} from "@/hooks/auth/use-account";
import { authClient } from "@/lib/auth-client";
import { GlobeIcon, WorkflowIcon } from "@/lib/icons";
import { captureException } from "@/lib/sentry";

const PROVIDER_LABELS: Record<string, string> = {
  google: "Google",
  github: "GitHub",
  credential: "Email & Password",
};

const LINKABLE_PROVIDERS = ["google", "github"] as const;
const PROVIDER_ICONS = {
  google: GlobeIcon,
  github: WorkflowIcon,
} as const;

const LinkedAccounts = () => {
  const { data: accounts = [], isLoading } = useQuery(accountsQueryOptions());
  const unlinkAccount = useUnlinkAccount();
  const [linkingProvider, setLinkingProvider] = useState<string | null>(null);

  const linkedProviders = new Set(accounts.map((a) => a.providerId));

  const handleLink = async (provider: string) => {
    setLinkingProvider(provider);
    try {
      await authClient.linkSocial({
        provider: provider as "google" | "github",
        callbackURL: "/app/settings",
      });
    } catch (error) {
      captureException(error);
      toast.error(`Failed to link ${PROVIDER_LABELS[provider] ?? provider}.`);
      setLinkingProvider(null);
    }
  };

  const handleUnlink = async (provider: string) => {
    const remainingAccounts = accounts.filter((a) => a.providerId !== provider);
    if (remainingAccounts.length === 0) {
      toast.error(
        "You must have at least one sign-in method. Add another before unlinking."
      );
      return;
    }

    try {
      await unlinkAccount.mutateAsync(provider);
      toast.success(`${PROVIDER_LABELS[provider] ?? provider} unlinked.`);
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : `Failed to unlink ${PROVIDER_LABELS[provider] ?? provider}.`
      );
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Linked accounts</CardTitle>
        <CardDescription>
          Manage the sign-in methods connected to your account.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading && (
          <div className="flex items-center gap-2 text-muted-foreground text-sm">
            <Spinner />
            Loading accounts...
          </div>
        )}
        {!isLoading && (
          <ItemGroup>
            {LINKABLE_PROVIDERS.map((provider) => {
              const isLinked = linkedProviders.has(provider);
              const isUnlinking =
                unlinkAccount.isPending && unlinkAccount.variables === provider;
              const isProcessing = linkingProvider === provider || isUnlinking;

              return (
                <Item key={provider} variant="outline">
                  <ItemMedia variant="icon">
                    <HugeiconsIcon
                      aria-hidden="true"
                      icon={PROVIDER_ICONS[provider]}
                      size={18}
                    />
                  </ItemMedia>
                  <ItemContent>
                    <ItemTitle>{PROVIDER_LABELS[provider]}</ItemTitle>
                    <ItemDescription>
                      {isLinked ? "Connected" : "Not connected"}
                    </ItemDescription>
                  </ItemContent>

                  <ItemActions>
                    {isLinked ? (
                      <Button
                        disabled={isProcessing || accounts.length <= 1}
                        onClick={() => handleUnlink(provider)}
                        variant="outline"
                      >
                        {isProcessing ? <Spinner size="xs" /> : null}
                        Unlink
                      </Button>
                    ) : (
                      <Button
                        disabled={isProcessing}
                        onClick={() => handleLink(provider)}
                      >
                        {isProcessing ? <Spinner size="xs" /> : null}
                        Link
                      </Button>
                    )}
                  </ItemActions>
                </Item>
              );
            })}
          </ItemGroup>
        )}
      </CardContent>
    </Card>
  );
};

export default LinkedAccounts;
