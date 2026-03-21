import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Input } from "@strait/ui/components/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@strait/ui/components/table";
import { toast } from "@strait/ui/components/toast/index";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { CopyToClipboard } from "react-copy-to-clipboard";
import {
  referralsQueryOptions,
  useActivateReferral,
  useCreateReferralCode,
} from "@/hooks/billing/use-referrals";
import { formatMicroUsd } from "@/lib/format";
import { CheckIcon, LoadingIcon, PlusIcon } from "@/lib/icons";

const StatusBadgeForReferral = ({ status }: { status: string }) => {
  if (status === "activated") {
    return <Badge variant="success-light">Activated</Badge>;
  }
  if (status === "pending") {
    return <Badge variant="info-light">Pending</Badge>;
  }
  return <Badge variant="secondary">{status}</Badge>;
};

const ReferralProgram = () => {
  const { data: referralData } = useQuery(referralsQueryOptions());
  const createCode = useCreateReferralCode();
  const activateReferral = useActivateReferral();
  const [activateCode, setActivateCode] = useState("");

  const handleCopyLink = () => {
    toast.success("Referral link copied to clipboard");
  };

  const handleGenerateCode = () => {
    createCode.mutate(undefined, {
      onSuccess: () => {
        toast.success("Referral code generated!");
      },
      onError: () => {
        toast.error("Failed to generate referral code");
      },
    });
  };

  const handleActivateCode = () => {
    if (!activateCode.trim()) {
      return;
    }
    const code = activateCode.trim();
    activateReferral.mutateAsync(code).then(
      () => {
        toast.success("Referral code activated!");
        setActivateCode("");
      },
      () => {
        toast.error("Failed to activate referral code");
      }
    );
  };

  const referrals = referralData?.referrals ?? [];
  const hasCode = !!referralData?.code;

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="font-medium text-sm">
              Your Referral Code
            </CardTitle>
          </CardHeader>
          <CardContent>
            {hasCode ? (
              <div className="space-y-3">
                <div className="flex items-center gap-2">
                  <code className="flex-1 rounded-md border bg-muted px-3 py-2 font-mono text-sm">
                    {referralData.code}
                  </code>
                  <CopyToClipboard
                    onCopy={handleCopyLink}
                    text={`https://strait.dev/r/${referralData.code}`}
                  >
                    <Button size="sm" variant="outline">
                      Copy Link
                    </Button>
                  </CopyToClipboard>
                </div>
                <p className="text-muted-foreground text-xs">
                  Share your link: https://strait.dev/r/{referralData.code}
                </p>
              </div>
            ) : (
              <Button
                disabled={createCode.isPending}
                onClick={handleGenerateCode}
                size="sm"
              >
                {createCode.isPending ? (
                  <HugeiconsIcon
                    className="mr-1.5 size-4 animate-spin"
                    icon={LoadingIcon}
                  />
                ) : (
                  <HugeiconsIcon className="mr-1.5 size-4" icon={PlusIcon} />
                )}
                Generate Referral Code
              </Button>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="font-medium text-sm">
              Credits Earned
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="font-normal text-2xl tabular-nums">
              {formatMicroUsd(referralData?.total_credit_microusd ?? 0)}
            </p>
            <p className="mt-1 text-muted-foreground text-xs">
              Total credit earned from referrals
            </p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="font-medium text-sm">
            Activate a Referral Code
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2">
            <Input
              className="max-w-xs"
              onChange={(e) => setActivateCode(e.target.value)}
              placeholder="Enter referral code"
              value={activateCode}
            />
            <Button
              disabled={!activateCode.trim() || activateReferral.isPending}
              onClick={handleActivateCode}
              size="sm"
            >
              {activateReferral.isPending ? (
                <HugeiconsIcon
                  className="mr-1.5 size-4 animate-spin"
                  icon={LoadingIcon}
                />
              ) : (
                <HugeiconsIcon className="mr-1.5 size-4" icon={CheckIcon} />
              )}
              Activate
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="font-medium text-sm">
            Referral History
          </CardTitle>
        </CardHeader>
        <CardContent>
          {referrals.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              No referrals yet. Share your referral code to earn credits.
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Status</TableHead>
                  <TableHead>Referred Email</TableHead>
                  <TableHead className="text-right">Credit</TableHead>
                  <TableHead className="text-right">Date</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {referrals.map((referral) => (
                  <TableRow key={referral.id}>
                    <TableCell>
                      <StatusBadgeForReferral status={referral.status} />
                    </TableCell>
                    <TableCell>{referral.referred_email || "-"}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatMicroUsd(referral.credit_microusd)}
                    </TableCell>
                    <TableCell className="text-right text-muted-foreground">
                      {referral.activated_at
                        ? new Date(referral.activated_at).toLocaleDateString()
                        : new Date(referral.created_at).toLocaleDateString()}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
};

export default ReferralProgram;
