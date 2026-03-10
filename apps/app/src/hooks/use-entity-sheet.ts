import { useCallback, useState } from "react";

/**
 * Hook for managing entity sheet state
 *
 * Provides consistent state management for entity preview sheets.
 * Handles open/close state and selected entity tracking.
 *
 * @template T - The type of entity
 * @returns Object with sheet state and control functions
 *
 * @example
 * const customerSheet = useEntitySheet<Customer>();
 *
 * // Open sheet with entity
 * customerSheet.open(customer);
 *
 * // Close sheet
 * customerSheet.close();
 *
 * // Use in component
 * <EntitySheet
 *   open={customerSheet.isOpen}
 *   onOpenChange={customerSheet.setOpen}
 *   {...props}
 * >
 *   {customerSheet.entity && (
 *     <CustomerSheetContent customer={customerSheet.entity} />
 *   )}
 * </EntitySheet>
 */
const ENTITY_SHEET_ANIMATION_DELAY = 200;

export const useEntitySheet = <TData = any>() => {
  const [isOpen, setIsOpen] = useState(false);
  const [entity, setEntity] = useState<TData | null>(null);

  const open = useCallback((entityData: TData) => {
    setEntity(entityData);
    setIsOpen(true);
  }, []);

  const close = useCallback(() => {
    setIsOpen(false);
    // Small delay to allow animation to complete before clearing entity
    setTimeout(() => {
      setEntity(null);
    }, ENTITY_SHEET_ANIMATION_DELAY);
  }, []);

  const handleOpenChange = useCallback(
    (openState: boolean) => {
      if (openState) {
        setIsOpen(true);
      } else {
        close();
      }
    },
    [close]
  );

  return {
    isOpen,
    entity,
    open,
    close,
    setOpen: handleOpenChange,
  } as const;
};

/**
 * Hook for managing entity tabs navigation
 *
 * Provides state management for tabbed entity detail views.
 * Handles active tab state with URL synchronization support.
 *
 * @param defaultTab - The default active tab
 * @param tabs - Array of available tab configurations
 * @returns Object with tab state and navigation functions
 *
 * @example
 * const tabs = useEntityTabs('overview', [
 *   { id: 'overview', label: 'Overview' },
 *   { id: 'orders', label: 'Orders' },
 *   { id: 'analytics', label: 'Analytics' }
 * ]);
 *
 * <Tabs value={tabs.activeTab} onValueChange={tabs.setActiveTab}>
 *   {tabs.tabsList.map(tab => (
 *     <TabsTrigger key={tab.id} value={tab.id}>
 *       {tab.label}
 *     </TabsTrigger>
 *   ))}
 * </Tabs>
 */
export type TabConfig = {
  id: string;
  label: string;
  disabled?: boolean;
  badge?: string | number;
};

export const useEntityTabs = (
  defaultTab = "overview",
  tabs: TabConfig[] = []
) => {
  const [activeTab, setActiveTab] = useState(defaultTab);

  const switchToTab = useCallback(
    (tabId: string) => {
      const tab = tabs.find((t) => t.id === tabId);
      if (tab && !tab.disabled) {
        setActiveTab(tabId);
      }
    },
    [tabs]
  );

  const getTabById = useCallback(
    (tabId: string) => tabs.find((t) => t.id === tabId),
    [tabs]
  );

  const isTabActive = useCallback(
    (tabId: string) => activeTab === tabId,
    [activeTab]
  );

  const getActiveTab = useCallback(
    () => getTabById(activeTab),
    [activeTab, getTabById]
  );

  return {
    activeTab,
    setActiveTab: switchToTab,
    tabsList: tabs,
    getTabById,
    isTabActive,
    getActiveTab,
  } as const;
};
