"use client";

import { AnimatePresence, motion } from "motion/react";
import { useCallback, useState } from "react";
import MockBrowserWindow from "@/components/magicui/mock-browser-window.tsx";
import { SPRING_SNAPPY } from "@/lib/motion.ts";
import HeroApprovalTab from "./tabs/hero-approval-tab.tsx";
import HeroObservabilityTab from "./tabs/hero-observability-tab.tsx";
import HeroRetriesTab from "./tabs/hero-retries-tab.tsx";
import HeroWorkflowsTab from "./tabs/hero-workflows-tab.tsx";

const TABS = [
  { label: "Workflows", id: "workflows" },
  { label: "Retries & DLQ", id: "retries" },
  { label: "Approval Gates", id: "approvals" },
  { label: "Observability", id: "observability" },
] as const;

const TAB_COMPONENTS: Record<string, React.FC> = {
  workflows: HeroWorkflowsTab,
  retries: HeroRetriesTab,
  approvals: HeroApprovalTab,
  observability: HeroObservabilityTab,
};

const HeroProductPreview = () => {
  const [activeTab, setActiveTab] = useState(0);
  const currentTab = TABS[activeTab] ?? TABS[0];
  const TabContent = TAB_COMPONENTS[currentTab.id] ?? HeroWorkflowsTab;

  const handleTabKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      let next = activeTab;
      if (e.key === "ArrowRight") {
        next = (activeTab + 1) % TABS.length;
      } else if (e.key === "ArrowLeft") {
        next = (activeTab - 1 + TABS.length) % TABS.length;
      } else {
        return;
      }
      e.preventDefault();
      setActiveTab(next);
      const btn =
        e.currentTarget.querySelectorAll<HTMLButtonElement>('[role="tab"]')[
          next
        ];
      btn?.focus();
    },
    [activeTab]
  );

  return (
    <MockBrowserWindow
      className="shadow-2xl shadow-black/20"
      url="strait dashboard"
    >
      {/* Tab bar */}
      <div className="border-border/40 border-b bg-muted/20 px-4 py-2">
        <div
          aria-label="Feature preview"
          className="flex items-center gap-1"
          onKeyDown={handleTabKeyDown}
          role="tablist"
        >
          {TABS.map((tab, i) => (
            <button
              aria-controls={`hero-preview-${tab.id}`}
              aria-selected={activeTab === i}
              className={`rounded-md px-3 py-1.5 font-medium text-xs transition-colors ${
                activeTab === i
                  ? "bg-primary/15 text-primary"
                  : "text-muted-foreground/60 hover:text-muted-foreground"
              }`}
              id={`hero-preview-tab-${tab.id}`}
              key={tab.id}
              onClick={() => setActiveTab(i)}
              role="tab"
              tabIndex={activeTab === i ? 0 : -1}
              type="button"
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      {/* Tab content */}
      <div
        aria-labelledby={`hero-preview-tab-${currentTab.id}`}
        className="h-[420px] overflow-hidden"
        id={`hero-preview-${currentTab.id}`}
        role="tabpanel"
      >
        <AnimatePresence mode="wait">
          <motion.div
            animate={{ opacity: 1, y: 0 }}
            className="h-full"
            exit={{ opacity: 0, y: -6 }}
            initial={{ opacity: 0, y: 6 }}
            key={currentTab.id}
            transition={SPRING_SNAPPY}
          >
            <TabContent />
          </motion.div>
        </AnimatePresence>
      </div>
    </MockBrowserWindow>
  );
};

export default HeroProductPreview;
