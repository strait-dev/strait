import { useSyncExternalStore } from "react";

const subscribe = (onStoreChange: () => void) => {
  onStoreChange();
  return () => undefined;
};

const getClientSnapshot = () => true;
const getServerSnapshot = () => false;

export function useIsHydrated() {
  return useSyncExternalStore(subscribe, getClientSnapshot, getServerSnapshot);
}
