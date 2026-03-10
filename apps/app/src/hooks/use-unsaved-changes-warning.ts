import { useBlocker, useRouter } from "@tanstack/react-router";
import { useCallback } from "react";

type UseUnsavedChangesWarningOptions = {
  /**
   * Whether the form has unsaved changes (typically formState.isDirty)
   */
  isDirty: boolean;
  /**
   * Optional: Callback to execute when user confirms discard
   */
  onDiscard?: () => void;
  /**
   * Optional: Whether the blocker is disabled (e.g., during submission)
   */
  disabled?: boolean;
};

type UseUnsavedChangesWarningReturn = {
  /**
   * Whether the blocker is currently active (showing dialog)
   */
  isBlocked: boolean;
  /**
   * Call to proceed with navigation (discard changes)
   */
  proceed: () => void;
  /**
   * Call to cancel navigation (stay on page)
   */
  reset: () => void;
};

export const useUnsavedChangesWarning = ({
  isDirty,
  onDiscard,
  disabled = false,
}: UseUnsavedChangesWarningOptions): UseUnsavedChangesWarningReturn => {
  const router = useRouter();

  const shouldBlock = isDirty && !disabled;

  const {
    status,
    proceed: blockerProceed,
    reset: blockerReset,
  } = useBlocker({
    shouldBlockFn: () => shouldBlock,
    withResolver: true,
    // Enable browser's native beforeunload dialog for tab close/refresh
    // Check router status to prevent double dialogs (known TanStack Router behavior)
    enableBeforeUnload: () => shouldBlock && router.state.status !== "pending",
  });

  const isBlocked = status === "blocked";

  const proceed = useCallback(() => {
    onDiscard?.();
    blockerProceed?.();
  }, [blockerProceed, onDiscard]);

  const reset = useCallback(() => {
    blockerReset?.();
  }, [blockerReset]);

  return {
    isBlocked,
    proceed,
    reset,
  };
};
