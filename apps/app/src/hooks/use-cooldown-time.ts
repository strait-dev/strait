import { useSyncExternalStore } from "react";
import { MILLISECONDS_PER_SECOND, TIMER_INTERVAL_MS } from "@/utils/constants";

const COOLDOWN_CHANGE_EVENT = "strait:cooldown-change";

function getRemainingCooldown(storageKey: string) {
  if (typeof window === "undefined") {
    return 0;
  }

  const storedCooldownEnd = window.localStorage.getItem(storageKey);
  if (!storedCooldownEnd) {
    return 0;
  }

  const cooldownEnd = Number.parseInt(storedCooldownEnd, 10);
  if (!Number.isFinite(cooldownEnd)) {
    return 0;
  }

  return Math.max(
    0,
    Math.ceil((cooldownEnd - Date.now()) / MILLISECONDS_PER_SECOND)
  );
}

function clearExpiredCooldown(storageKey: string) {
  if (typeof window !== "undefined" && getRemainingCooldown(storageKey) === 0) {
    window.localStorage.removeItem(storageKey);
  }
}

function emitCooldownChange() {
  if (typeof window !== "undefined") {
    window.dispatchEvent(new Event(COOLDOWN_CHANGE_EVENT));
  }
}

function subscribeToCooldown(storageKey: string, onStoreChange: () => void) {
  if (typeof window === "undefined") {
    return () => undefined;
  }

  const notify = () => {
    clearExpiredCooldown(storageKey);
    onStoreChange();
  };

  const timer = window.setInterval(notify, TIMER_INTERVAL_MS);
  window.addEventListener("storage", notify);
  window.addEventListener(COOLDOWN_CHANGE_EVENT, notify);

  return () => {
    window.clearInterval(timer);
    window.removeEventListener("storage", notify);
    window.removeEventListener(COOLDOWN_CHANGE_EVENT, notify);
  };
}

export function useCooldownTime(storageKey: string) {
  return useSyncExternalStore(
    (onStoreChange) => subscribeToCooldown(storageKey, onStoreChange),
    () => getRemainingCooldown(storageKey),
    () => 0
  );
}

export function startCooldown(storageKey: string, seconds: number) {
  if (typeof window === "undefined") {
    return;
  }

  window.localStorage.setItem(
    storageKey,
    String(Date.now() + seconds * MILLISECONDS_PER_SECOND)
  );
  emitCooldownChange();
}
