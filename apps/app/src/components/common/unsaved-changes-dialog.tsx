import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@strait/ui/components/alert-dialog.tsx";
import { buttonVariants } from "@strait/ui/components/button.tsx";

type UnsavedChangesDialogProps = {
  /**
   * Whether the dialog is open
   */
  open: boolean;
  /**
   * Called when user clicks "Discard changes"
   */
  onDiscard: () => void;
  /**
   * Called when user clicks "Cancel" (stay on page)
   */
  onCancel: () => void;
};

export const UnsavedChangesDialog = ({
  open,
  onDiscard,
  onCancel,
}: UnsavedChangesDialogProps) => {
  return (
    <AlertDialog onOpenChange={(isOpen) => !isOpen && onCancel()} open={open}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Unsaved changes</AlertDialogTitle>
          <AlertDialogDescription>
            You have unsaved changes that will be lost if you leave this page.
            Are you sure you want to discard your changes?
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel onClick={onCancel}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            className={buttonVariants({ variant: "destructive" })}
            onClick={onDiscard}
          >
            Discard changes
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
};
