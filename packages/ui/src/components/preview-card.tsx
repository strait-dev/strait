"use client";

import { PreviewCard as PreviewCardPrimitive } from "@base-ui/react/preview-card";
import type React from "react";

import { cn } from "../utils/index";

const PREVIEW_CARD_DELAY = 300;

function PreviewCard({
  ...props
}: React.ComponentProps<typeof PreviewCardPrimitive.Root>) {
  return <PreviewCardPrimitive.Root data-slot="preview-card" {...props} />;
}

function PreviewCardTrigger({
  delay = PREVIEW_CARD_DELAY,
  closeDelay = PREVIEW_CARD_DELAY,
  className,
  ...props
}: React.ComponentProps<typeof PreviewCardPrimitive.Trigger>) {
  return (
    <PreviewCardPrimitive.Trigger
      className={className}
      closeDelay={closeDelay}
      data-slot="preview-card-trigger"
      delay={delay}
      {...props}
    />
  );
}

function PreviewCardPortal({
  ...props
}: React.ComponentProps<typeof PreviewCardPrimitive.Portal>) {
  return (
    <PreviewCardPrimitive.Portal data-slot="preview-card-portal" {...props} />
  );
}

function PreviewCardPositioner({
  className,
  sideOffset = 8,
  ...props
}: React.ComponentProps<typeof PreviewCardPrimitive.Positioner>) {
  return (
    <PreviewCardPrimitive.Positioner
      className={cn("z-50", className)}
      data-slot="preview-card-positioner"
      sideOffset={sideOffset}
      {...props}
    />
  );
}

function PreviewCardPopup({
  className,
  ...props
}: React.ComponentProps<typeof PreviewCardPrimitive.Popup>) {
  return (
    <PreviewCardPrimitive.Popup
      className={cn(
        "w-80 origin-[var(--transform-origin)] rounded-md border bg-popover p-4 text-popover-foreground shadow-lg outline-hidden transition-[transform,scale,opacity] data-[ending-style]:scale-95 data-[starting-style]:scale-95 data-[ending-style]:opacity-0 data-[starting-style]:opacity-0",
        className
      )}
      data-slot="preview-card-popup"
      {...props}
    />
  );
}

function PreviewCardArrow({
  className,
  ...props
}: React.ComponentProps<typeof PreviewCardPrimitive.Arrow>) {
  return (
    <PreviewCardPrimitive.Arrow
      className={cn(
        "-my-px fill-popover drop-shadow-[0_1px_0_hsl(var(--border))]",
        className
      )}
      data-slot="preview-card-arrow"
      {...props}
    />
  );
}

function PreviewCardBackdrop({
  className,
  ...props
}: React.ComponentProps<typeof PreviewCardPrimitive.Backdrop>) {
  return (
    <PreviewCardPrimitive.Backdrop
      className={cn("fixed inset-0 z-40", className)}
      data-slot="preview-card-backdrop"
      {...props}
    />
  );
}

export {
  PreviewCard,
  PreviewCardArrow,
  PreviewCardBackdrop,
  PreviewCardPopup,
  PreviewCardPortal,
  PreviewCardPositioner,
  PreviewCardTrigger,
};
