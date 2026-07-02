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
import { EmptyMedia } from "@strait/ui/components/empty";
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
  const onUrlCleanupRef = useRef(onUrlCleanup);
  onUrlCleanupRef.current = onUrlCleanup;

  useEffect(() => {
    if (isNewSubscription) {
      setOpen(true);
      onUrlCleanupRef.current?.();
    } else if (
      isUpgrade &&
      timestamp &&
      !shownTimestampsRef.current.has(timestamp)
    ) {
      shownTimestampsRef.current.add(timestamp);
      setOpen(true);
      onUrlCleanupRef.current?.();
    }
  }, [isNewSubscription, isUpgrade, timestamp]);

  const handleClose = useCallback(() => {
    setOpen(false);
    onClose?.();
  }, [onClose]);

  return (
    <Credenza onOpenChange={setOpen} open={open}>
      <CredenzaContent className="sm:max-w-[500px]">
        <CredenzaHeader className="text-center">
          <div className="mb-6 flex justify-center">
            <EmptyMedia media="icon" size="lg" variant="success">
              <HugeiconsIcon className="size-12" icon={CheckCircle2Icon} />
            </EmptyMedia>
          </div>
          <CredenzaTitle className="text-balance text-2xl">
            {isNewSubscription ? "Welcome to Strait!" : null}
            {isUpgrade ? "Subscription updated!" : null}
          </CredenzaTitle>
          <CredenzaDescription className="mt-3 text-base text-muted-foreground">
            {isNewSubscription
              ? "Your subscription is now active. Your selected plan limits have been applied."
              : null}
            {isUpgrade
              ? "Your subscription has been updated successfully. Your new plan limits are active."
              : null}
          </CredenzaDescription>
        </CredenzaHeader>

        {/* Feature highlights */}
        <div className="px-6 pb-2">
          <div className="flex justify-center gap-3">
            <Badge iconLeft={SparklesIcon} size="lg" variant="success">
              Plan limits active
            </Badge>
            <Badge iconLeft={CreditCardIcon} size="lg" variant="info-light">
              Manage billing
            </Badge>
          </div>
        </div>

        <CredenzaFooter className="flex justify-center pt-4">
          <Button onClick={handleClose}>Start exploring</Button>
        </CredenzaFooter>
      </CredenzaContent>
    </Credenza>
  );
};

export default SubscriptionSuccessDialog;
