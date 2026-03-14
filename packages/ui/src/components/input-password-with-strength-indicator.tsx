"use client";

import {
  Cancel01Icon,
  Tick02Icon,
  ViewIcon,
  ViewOffIcon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useCallback, useEffect, useId, useMemo, useState } from "react";

import { cn } from "../utils/index";
import { Input } from "./input";
import { Label } from "./label";

const MAX_STRENGTH = 4;
const PERCENTAGE_TO_WIDTH = 100;

export type InputPasswordWithStrengthIndicatorProps = Omit<
  React.InputHTMLAttributes<HTMLInputElement>,
  "type"
> & {
  label?: string;
  error?: boolean;
  showStrengthIndicator?: boolean;
};

const requirements = [
  { regex: /.{8,}/, text: "At least 8 characters" },
  { regex: /[0-9]/, text: "At least 1 number" },
  { regex: /[a-z]/, text: "At least 1 lowercase letter" },
  { regex: /[A-Z]/, text: "At least 1 uppercase letter" },
] as const;

const getStrengthColor = (score: number) => {
  const SCORE_THRESHOLDS = {
    WEAK: 1,
    MEDIUM: 2,
    STRONG: 3,
  };

  if (score === 0) {
    return "bg-border";
  }
  if (score <= SCORE_THRESHOLDS.WEAK) {
    return "bg-red-500";
  }
  if (score <= SCORE_THRESHOLDS.MEDIUM) {
    return "bg-orange-500";
  }
  if (score === SCORE_THRESHOLDS.STRONG) {
    return "bg-amber-500";
  }
  return "bg-emerald-500";
};

const getStrengthText = (score: number) => {
  const SCORE_THRESHOLDS = {
    WEAK: 1,
    MEDIUM: 2,
    STRONG: 3,
  };

  if (score === 0) {
    return "Type your password";
  }
  if (score <= SCORE_THRESHOLDS.WEAK) {
    return "Weak password";
  }
  if (score === SCORE_THRESHOLDS.MEDIUM) {
    return "Medium password";
  }
  return "Strong password";
};

function InputPasswordWithStrengthIndicator({
  className,
  label,
  error,
  showStrengthIndicator = true,
  onChange,
  placeholder,
  value,
  ...props
}: InputPasswordWithStrengthIndicatorProps) {
  const passwordStrengthId = useId();
  const [password, setPassword] = useState<string>((value as string) || "");
  const [isVisible, setIsVisible] = useState<boolean>(false);
  const [isTouched, setIsTouched] = useState<boolean>(false);

  // Watch for changes in the value prop to sync with internal state
  // Only update if the value is explicitly set to empty string (form reset)
  useEffect(() => {
    if (value === "") {
      setPassword("");
      setIsTouched(false);
    }
  }, [value]);

  const toggleVisibility = useCallback(() => {
    setIsVisible((prevState) => !prevState);
  }, []);

  const strength = useMemo(
    () =>
      requirements.map((req) => ({
        met: req.regex.test(password),
        text: req.text,
      })),
    [password]
  );

  const strengthScore = useMemo(
    () => strength.filter((req) => req.met).length,
    [strength]
  );

  const strengthColor = useMemo(
    () => getStrengthColor(strengthScore),
    [strengthScore]
  );

  const strengthText = useMemo(
    () => getStrengthText(strengthScore),
    [strengthScore]
  );

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setPassword(e.target.value);
      onChange?.(e);
    },
    [onChange]
  );

  const { onFocus } = props;

  const handleFocus = useCallback(
    (e: React.FocusEvent<HTMLInputElement>) => {
      setIsTouched(true);
      onFocus?.(e);
    },
    [onFocus]
  );

  const inputProps = useMemo(
    () => ({
      type: isVisible ? "text" : "password",
      value: password,
      onChange: handleChange,
      placeholder,
      onFocus: handleFocus,
      className: cn("pe-9", error && "border-destructive", className),
      "aria-invalid": (() => {
        if (error) {
          return true;
        }
        if (isTouched && password) {
          return strengthScore < MAX_STRENGTH;
        }
        return false;
      })(),
      "aria-describedby": passwordStrengthId,
    }),
    [
      isVisible,
      password,
      handleChange,
      handleFocus,
      error,
      className,
      strengthScore,
      placeholder,
      isTouched,
      passwordStrengthId,
    ]
  );

  const buttonProps = useMemo(
    () => ({
      className:
        "absolute inset-y-0 end-0 flex h-full w-9 items-center justify-center rounded-e-lg text-muted-foreground/80 outline-offset-2 transition-colors hover:text-foreground focus:z-10 focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring/70 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50",
      type: "button" as const,
      onClick: toggleVisibility,
      "aria-label": isVisible ? "Hide password" : "Show password",
      "aria-pressed": isVisible,
      "aria-controls": "password",
    }),
    [isVisible, toggleVisibility]
  );

  const shouldShowIndicator = showStrengthIndicator && isTouched && password;

  return (
    <div
      className="flex flex-col gap-2"
      data-slot="input-password-with-strength-indicator"
    >
      {label ? (
        <Label
          className="font-medium text-foreground text-sm"
          data-slot="label"
          htmlFor={props.id}
        >
          {label}
        </Label>
      ) : null}
      <div className="relative" data-slot="input-container">
        <Input data-slot="input" {...inputProps} />
        <button data-slot="visibility-toggle" {...buttonProps}>
          {isVisible ? (
            <HugeiconsIcon
              aria-hidden="true"
              className="size-4"
              icon={ViewOffIcon}
            />
          ) : (
            <HugeiconsIcon
              aria-hidden="true"
              className="size-4"
              icon={ViewIcon}
            />
          )}
        </button>
      </div>

      {shouldShowIndicator ? (
        <div
          className="overflow-hidden transition-all duration-200 ease-in-out"
          data-slot="strength-indicator"
        >
          <div
            aria-label="Password strength"
            aria-valuemax={4}
            aria-valuemin={0}
            aria-valuenow={strengthScore}
            className="fade-in slide-in-from-top-1 mt-3 mb-4 h-1 w-full animate-in overflow-hidden rounded-md bg-border"
            data-slot="strength-progress"
            role="progressbar"
          >
            <div
              className={cn(
                "h-full transition-all duration-500 ease-out",
                strengthColor
              )}
              data-slot="strength-progress-bar"
              style={{
                width: `${(strengthScore / MAX_STRENGTH) * PERCENTAGE_TO_WIDTH}%`,
              }}
            />
          </div>

          <div
            className="fade-in slide-in-from-top-2 animate-in"
            data-slot="strength-requirements"
          >
            <p
              className="mb-2 font-medium text-foreground text-sm"
              data-slot="strength-text"
              id={passwordStrengthId}
            >
              {strengthText}. Must contain:
            </p>

            <ul
              aria-label="Password requirements"
              className="flex flex-col gap-1.5"
              data-slot="strength-requirements-list"
            >
              {strength.map((req) => (
                <li
                  className="flex items-center gap-2.5"
                  data-slot="strength-requirement"
                  key={req.text}
                >
                  {req.met ? (
                    <HugeiconsIcon
                      aria-hidden="true"
                      className="size-4 text-emerald-500"
                      icon={Tick02Icon}
                    />
                  ) : (
                    <HugeiconsIcon
                      aria-hidden="true"
                      className="size-4 text-muted-foreground/80"
                      icon={Cancel01Icon}
                    />
                  )}
                  <span
                    className={cn(
                      "text-sm",
                      req.met ? "text-emerald-600" : "text-muted-foreground"
                    )}
                    data-slot="strength-requirement-text"
                  >
                    {req.text}
                    <span className="sr-only">
                      {req.met
                        ? " - Requirement met"
                        : " - Requirement not met"}
                    </span>
                  </span>
                </li>
              ))}
            </ul>
          </div>
        </div>
      ) : null}
    </div>
  );
}

export { InputPasswordWithStrengthIndicator };
