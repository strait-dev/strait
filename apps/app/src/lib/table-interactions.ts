import type { MouseEvent } from "react";

const INTERACTIVE_ROW_TARGET_SELECTOR =
  'a, button, input, select, textarea, [role="button"], [role="checkbox"], [data-no-row-click]';

export function stopInteractiveRowClick(event: MouseEvent<HTMLElement>) {
  const target = event.target;
  if (!(target instanceof HTMLElement)) {
    return;
  }
  if (target.closest(INTERACTIVE_ROW_TARGET_SELECTOR)) {
    event.stopPropagation();
  }
}
