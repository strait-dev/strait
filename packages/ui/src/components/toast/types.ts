import type { ExternalToast } from "sonner";

export interface ToastAction {
  label: string;
  onClick: () => void;
}

export interface BaseToastOptions extends Omit<ExternalToast, "action"> {
  action?: ToastAction;
  description?: string;
  dismissible?: boolean;
  duration?: number;
}

export interface PromiseToastOptions<T> {
  description?: {
    loading?: string;
    success?: string | ((data: T) => string);
    error?: string | ((error: unknown) => string);
  };
  error: string | ((error: unknown) => string);
  loading: string;
  success: string | ((data: T) => string);
}

export interface ConfirmToastOptions {
  cancelLabel?: string;
  confirmLabel?: string;
  description?: string;
  duration?: number;
  onCancel?: () => void;
  onConfirm: () => void | Promise<void>;
  onError?: (error: unknown) => void;
  variant?: "destructive" | "default";
}

export interface ToastContentProps {
  action?: ToastAction;
  className?: string;
  copyable?: boolean;
  description?: string;
  dismissible?: boolean;
  onDismiss?: () => void;
  title: string;
}

export interface Toast {
  confirm: (title: string, options: ConfirmToastOptions) => string | number;
  dismiss: (id?: string | number) => void;
  error: (title: string, options?: BaseToastOptions) => string | number;
  info: (title: string, options?: BaseToastOptions) => string | number;
  loading: (
    title: string,
    options?: Omit<BaseToastOptions, "action">
  ) => string | number;
  promise: <T>(
    promise: Promise<T>,
    options: PromiseToastOptions<T>
  ) => Promise<T>;
  success: (title: string, options?: BaseToastOptions) => string | number;
  warning: (title: string, options?: BaseToastOptions) => string | number;
}
