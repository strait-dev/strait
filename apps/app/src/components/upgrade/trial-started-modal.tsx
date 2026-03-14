import { Tick02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@strait/ui/components/dialog";

type TrialStartedModalProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export const TrialStartedModal = ({
  open,
  onOpenChange,
}: TrialStartedModalProps) => {
  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader className="text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <HugeiconsIcon className="h-6 w-6 text-primary" icon={Tick02Icon} />
          </div>
          <DialogTitle className="text-center">Welcome to Strait!</DialogTitle>
          <DialogDescription className="text-center">
            Your 14-day free trial has started. Explore all features and see how
            Strait can help grow your business.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3 py-4">
          <div className="flex items-start gap-3">
            <div className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-primary/10">
              <HugeiconsIcon
                className="h-3 w-3 text-primary"
                icon={Tick02Icon}
              />
            </div>
            <p className="text-muted-foreground text-sm">
              Full access to all features during your trial
            </p>
          </div>
          <div className="flex items-start gap-3">
            <div className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-primary/10">
              <HugeiconsIcon
                className="h-3 w-3 text-primary"
                icon={Tick02Icon}
              />
            </div>
            <p className="text-muted-foreground text-sm">
              No credit card required until trial ends
            </p>
          </div>
          <div className="flex items-start gap-3">
            <div className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-primary/10">
              <HugeiconsIcon
                className="h-3 w-3 text-primary"
                icon={Tick02Icon}
              />
            </div>
            <p className="text-muted-foreground text-sm">
              Cancel anytime before your trial ends
            </p>
          </div>
        </div>

        <DialogFooter>
          <Button className="w-full" onClick={() => onOpenChange(false)}>
            Get started
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};
