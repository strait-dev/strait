import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Credenza,
  CredenzaContent,
  CredenzaDescription,
  CredenzaFooter,
  CredenzaHeader,
  CredenzaTitle,
} from "@strait/ui/components/credenza";
import { useCallback, useEffect, useRef, useState } from "react";
import { CheckCircle2Icon, CreditCardIcon, SparklesIcon } from "@/lib/icons";

type SubscriptionSuccessDialogProps = {
  timestamp?: string;
  checkoutId?: string;
  onClose?: () => void;
  onUrlCleanup?: () => void;
  // URL-based success indicators
  isNewSubscription?: boolean;
  isUpgrade?: boolean;
};

const SubscriptionSuccessDialog = ({
  timestamp,
  checkoutId: _checkoutId,
  onClose,
  onUrlCleanup,
  isNewSubscription = false,
  isUpgrade = false,
}: SubscriptionSuccessDialogProps) => {
  const [open, setOpen] = useState(false);
  const shownTimestampsRef = useRef(new Set<string>());

  // Handle showing the dialog when subscription is upgraded or created
  useEffect(() => {
    const shouldShow = isNewSubscription || isUpgrade;

    if (shouldShow) {
      if (isNewSubscription) {
        setOpen(true);
      } else if (
        isUpgrade &&
        timestamp &&
        !shownTimestampsRef.current.has(timestamp)
      ) {
        // Use timestamp to prevent showing the same upgrade event multiple times
        shownTimestampsRef.current.add(timestamp);
        setOpen(true);
      }
    }
  }, [isNewSubscription, isUpgrade, timestamp]);

  // Notify parent to clean URL when dialog is shown
  useEffect(() => {
    if (open && (isNewSubscription || isUpgrade)) {
      onUrlCleanup?.();
    }
  }, [open, isNewSubscription, isUpgrade, onUrlCleanup]);

  // Handle dialog close
  const handleClose = useCallback(() => {
    setOpen(false);
    onClose?.();
  }, [onClose]);

  return (
    <Credenza onOpenChange={setOpen} open={open}>
      <CredenzaContent className="sm:max-w-[500px]">
        <CredenzaHeader className="text-center">
          <div className="mb-6 flex justify-center">
            <div className="rounded-lg bg-green-100 p-4 dark:bg-green-900/20">
              <HugeiconsIcon
                className="size-12 text-green-600 dark:text-green-400"
                icon={CheckCircle2Icon}
              />
            </div>
          </div>
          <CredenzaTitle className="text-balance text-2xl">
            {isNewSubscription ? "Welcome to Strait!" : null}
            {isUpgrade ? "Subscription Updated!" : null}
          </CredenzaTitle>
          <CredenzaDescription className="mt-3 text-base text-muted-foreground">
            {isNewSubscription
              ? "Your subscription is now active! You have access to all premium features to help grow your business."
              : null}
            {isUpgrade
              ? "Your subscription has been updated successfully. Enjoy your enhanced features and capabilities!"
              : null}
          </CredenzaDescription>
        </CredenzaHeader>

        {/* Feature highlights */}
        <div className="px-6 pb-2">
          <div className="flex justify-center gap-3">
            <Badge
              className="flex items-center gap-1 px-3 py-1"
              variant="success"
            >
              <HugeiconsIcon className="size-3" icon={SparklesIcon} />
              Premium Features
            </Badge>
            <Badge
              className="flex items-center gap-1 px-3 py-1"
              variant="info-light"
            >
              <HugeiconsIcon className="size-3" icon={CreditCardIcon} />
              Manage Billing
            </Badge>
          </div>
        </div>

        <CredenzaFooter className="flex justify-center pt-4">
          <Button className="px-8" onClick={handleClose} size="lg">
            Start Exploring
          </Button>
        </CredenzaFooter>
      </CredenzaContent>
    </Credenza>
  );
};

export default SubscriptionSuccessDialog;
