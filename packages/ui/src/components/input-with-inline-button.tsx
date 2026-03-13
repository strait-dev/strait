import { cn } from "../utils/index.ts";
import { Button } from "./button.tsx";
import { Input } from "./input.tsx";

type InputWithInlineButtonProps = React.ComponentProps<"input"> & {
  button?: React.ReactNode;
  buttonText?: string;
  onButtonClick?: () => void;
  buttonClassName?: string;
  buttonAriaLabel?: string;
  wrapperClassName?: string;
  buttonType?: "button" | "submit" | "reset";
};

function InputWithInlineButton({
  button,
  buttonText,
  onButtonClick,
  buttonClassName,
  buttonAriaLabel,
  wrapperClassName,
  buttonType = "submit",
  className,
  ...props
}: InputWithInlineButtonProps) {
  return (
    <div
      className={cn("flex rounded-md shadow-xs", wrapperClassName)}
      data-slot="input-with-inline-button"
    >
      <Input
        className={cn(
          "-me-px flex-1 rounded-e-none shadow-none focus-visible:z-10",
          className
        )}
        data-slot="input"
        {...props}
      />

      {button ? (
        // Custom button/component approach
        <div
          className={cn("rounded-s-none", buttonClassName)}
          data-slot="button-container"
        >
          {button}
        </div>
      ) : (
        // Text-based button approach
        <Button
          aria-label={buttonAriaLabel || buttonText}
          className={cn("rounded-s-none", buttonClassName)}
          data-slot="button"
          onClick={onButtonClick}
          type={buttonType}
          variant="outline"
        >
          {buttonText}
        </Button>
      )}
    </div>
  );
}

export { InputWithInlineButton };
export type { InputWithInlineButtonProps };
