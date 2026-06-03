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
import { EmptyMedia } from "@strait/ui/components/empty";
import { CheckIcon } from "@/lib/icons";

type TrialStartedModalProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const TrialStartedModal = ({ open, onOpenChange }: TrialStartedModalProps) => (
  <Dialog onOpenChange={onOpenChange} open={open}>
    <DialogContent className="sm:max-w-md">
      <DialogHeader className="text-center">
        <EmptyMedia
          className="mx-auto mb-4"
          media="icon"
          size="lg"
          variant="success"
        >
          <HugeiconsIcon className="size-6 text-foreground" icon={CheckIcon} />
        </EmptyMedia>
        <DialogTitle className="text-center">Welcome to Strait!</DialogTitle>
        <DialogDescription className="text-center">
          Your 14-day free trial has started. Explore all features and see how
          Strait can help grow your business.
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-3 py-4">
        <div className="flex items-start gap-3">
          <EmptyMedia
            className="mt-0.5"
            media="icon"
            size="sm"
            variant="success"
          >
            <HugeiconsIcon
              className="size-3 text-foreground"
              icon={CheckIcon}
            />
          </EmptyMedia>
          <p className="text-muted-foreground text-sm">
            Full access to all features during your trial
          </p>
        </div>
        <div className="flex items-start gap-3">
          <EmptyMedia
            className="mt-0.5"
            media="icon"
            size="sm"
            variant="success"
          >
            <HugeiconsIcon
              className="size-3 text-foreground"
              icon={CheckIcon}
            />
          </EmptyMedia>
          <p className="text-muted-foreground text-sm">
            No credit card required until trial ends
          </p>
        </div>
        <div className="flex items-start gap-3">
          <EmptyMedia
            className="mt-0.5"
            media="icon"
            size="sm"
            variant="success"
          >
            <HugeiconsIcon
              className="size-3 text-foreground"
              icon={CheckIcon}
            />
          </EmptyMedia>
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

export default TrialStartedModal;
