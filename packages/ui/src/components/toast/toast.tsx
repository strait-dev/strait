import { toast as sonnerToast } from "sonner";
import { ToastConfirm } from "./toast-confirm";
import { ToastError } from "./toast-error";
import { ToastInfo } from "./toast-info";
import { ToastLoading } from "./toast-loading";
import { ToastSuccess } from "./toast-success";
import { ToastWarning } from "./toast-warning";
import type { PromiseToastOptions, Toast } from "./types";

const DEFAULT_SUCCESS_DURATION = 4000;
const DEFAULT_ERROR_DURATION = 5000;
const DEFAULT_WARNING_DURATION = 5000;
const DEFAULT_INFO_DURATION = 4000;

export const toast: Toast = {
  success: (title, options) =>
    sonnerToast.custom(
      (id) => (
        <ToastSuccess
          action={options?.action}
          description={options?.description}
          dismissible={options?.dismissible}
          onDismiss={() => sonnerToast.dismiss(id)}
          title={title}
        />
      ),
      { duration: options?.duration ?? DEFAULT_SUCCESS_DURATION }
    ),

  error: (title, options) =>
    sonnerToast.custom(
      (id) => (
        <ToastError
          action={options?.action}
          description={options?.description}
          dismissible={options?.dismissible}
          onDismiss={() => sonnerToast.dismiss(id)}
          title={title}
        />
      ),
      { duration: options?.duration ?? DEFAULT_ERROR_DURATION }
    ),

  warning: (title, options) =>
    sonnerToast.custom(
      (id) => (
        <ToastWarning
          action={options?.action}
          description={options?.description}
          dismissible={options?.dismissible}
          onDismiss={() => sonnerToast.dismiss(id)}
          title={title}
        />
      ),
      { duration: options?.duration ?? DEFAULT_WARNING_DURATION }
    ),

  info: (title, options) =>
    sonnerToast.custom(
      (id) => (
        <ToastInfo
          action={options?.action}
          description={options?.description}
          dismissible={options?.dismissible}
          onDismiss={() => sonnerToast.dismiss(id)}
          title={title}
        />
      ),
      { duration: options?.duration ?? DEFAULT_INFO_DURATION }
    ),

  loading: (title, options) =>
    sonnerToast.custom(
      (id) => (
        <ToastLoading
          description={options?.description}
          onDismiss={() => sonnerToast.dismiss(id)}
          title={title}
        />
      ),
      { duration: Number.POSITIVE_INFINITY }
    ),

  promise: async <T,>(
    promise: Promise<T>,
    options: PromiseToastOptions<T>
  ): Promise<T> => {
    const id = sonnerToast.custom(
      (toastId) => (
        <ToastLoading
          description={options.description?.loading}
          onDismiss={() => sonnerToast.dismiss(toastId)}
          title={options.loading}
        />
      ),
      { duration: Number.POSITIVE_INFINITY }
    );

    try {
      const result = await promise;

      const successTitle =
        typeof options.success === "function"
          ? options.success(result)
          : options.success;

      const successDescription =
        typeof options.description?.success === "function"
          ? options.description.success(result)
          : options.description?.success;

      sonnerToast.custom(
        (toastId) => (
          <ToastSuccess
            description={successDescription}
            onDismiss={() => sonnerToast.dismiss(toastId)}
            title={successTitle}
          />
        ),
        { id, duration: DEFAULT_SUCCESS_DURATION }
      );

      return result;
    } catch (error) {
      const errorTitle =
        typeof options.error === "function"
          ? options.error(error)
          : options.error;

      const errorDescription =
        typeof options.description?.error === "function"
          ? options.description.error(error)
          : options.description?.error;

      sonnerToast.custom(
        (toastId) => (
          <ToastError
            description={errorDescription}
            onDismiss={() => sonnerToast.dismiss(toastId)}
            title={errorTitle}
          />
        ),
        { id, duration: DEFAULT_ERROR_DURATION }
      );

      throw error;
    }
  },

  confirm: (title, options) =>
    sonnerToast.custom(
      (id) => (
        <ToastConfirm
          cancelLabel={options.cancelLabel}
          confirmLabel={options.confirmLabel}
          description={options.description}
          onCancel={options.onCancel}
          onConfirm={options.onConfirm}
          onDismiss={() => sonnerToast.dismiss(id)}
          title={title}
          variant={options.variant}
        />
      ),
      { duration: options.duration ?? Number.POSITIVE_INFINITY }
    ),

  dismiss: sonnerToast.dismiss,
};
