"use client";

import { AnimatePresence, motion } from "motion/react";
import { useCallback, useEffect, useRef, useState } from "react";
import { SPRING_SNAPPY } from "@/lib/motion.ts";
import { CODE_TABS, type CodeToken } from "./hero-data.ts";

const CopyButton = ({ text }: { text: string }) => {
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  useEffect(() => {
    return () => clearTimeout(timerRef.current);
  }, []);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => setCopied(false), 2000);
  }, [text]);

  return (
    <button
      aria-label="Copy code to clipboard"
      className="rounded-md p-1.5 text-muted-foreground/50 transition-colors hover:bg-foreground/5 hover:text-muted-foreground"
      onClick={handleCopy}
      type="button"
    >
      {copied ? (
        <svg
          className="size-4"
          fill="none"
          stroke="currentColor"
          strokeWidth={2}
          viewBox="0 0 24 24"
        >
          <path
            d="M20 6L9 17l-5-5"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      ) : (
        <svg
          className="size-4"
          fill="none"
          stroke="currentColor"
          strokeWidth={2}
          viewBox="0 0 24 24"
        >
          <rect height="13" rx="2" width="13" x="9" y="9" />
          <path
            d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      )}
    </button>
  );
};

const CodeSnippet = ({ tokens }: { tokens: readonly CodeToken[] }) => (
  <pre className="overflow-x-auto font-mono text-sm leading-relaxed">
    <code>
      {tokens.map((token, i) => {
        let className = "text-foreground/80";
        if (token.kind === "keyword") {
          className = "text-primary";
        } else if (token.kind === "string") {
          className = "text-success";
        } else if (token.kind === "comment") {
          className = "text-muted-foreground";
        }
        return (
          <span
            className={className}
            key={`${String(i)}-${token.text.slice(0, 8)}`}
          >
            {token.text}
          </span>
        );
      })}
    </code>
  </pre>
);

function getPlainText(tokens: readonly CodeToken[]): string {
  return tokens.map((t) => t.text).join("");
}

const HeroCodeTabs = () => {
  const [activeTab, setActiveTab] = useState(0);

  const handleTabKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      let next = activeTab;
      if (e.key === "ArrowRight") {
        next = (activeTab + 1) % CODE_TABS.length;
      } else if (e.key === "ArrowLeft") {
        next = (activeTab - 1 + CODE_TABS.length) % CODE_TABS.length;
      } else if (e.key === "Home") {
        next = 0;
      } else if (e.key === "End") {
        next = CODE_TABS.length - 1;
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

  const currentTokens = CODE_TABS[activeTab]?.code ?? CODE_TABS[0].code;

  return (
    <div className="mx-auto max-w-2xl overflow-hidden rounded-xl border border-border/40 bg-card shadow-2xl shadow-black/10">
      {/* Tab bar */}
      <div className="flex items-center gap-1.5 border-border/40 border-b bg-muted/30 px-4 py-2.5">
        <div className="mr-3 flex gap-1.5">
          <div className="size-3 rounded-full bg-foreground/10" />
          <div className="size-3 rounded-full bg-foreground/10" />
          <div className="size-3 rounded-full bg-foreground/10" />
        </div>
        <div
          aria-label="Code language"
          className="flex items-center gap-1.5"
          onKeyDown={handleTabKeyDown}
          role="tablist"
        >
          {CODE_TABS.map((tab, i) => (
            <button
              aria-controls={`hero-tabpanel-${String(i)}`}
              aria-selected={activeTab === i}
              className={`rounded-md px-3 py-1 font-medium text-xs transition-colors ${
                activeTab === i
                  ? "bg-foreground/10 text-foreground"
                  : "text-muted-foreground hover:text-foreground"
              }`}
              id={`hero-tab-${String(i)}`}
              key={tab.label}
              onClick={() => setActiveTab(i)}
              role="tab"
              tabIndex={activeTab === i ? 0 : -1}
              type="button"
            >
              {tab.label}
            </button>
          ))}
        </div>
        <div className="ml-auto">
          <CopyButton text={getPlainText(currentTokens)} />
        </div>
      </div>
      {/* Code content */}
      <div
        aria-labelledby={`hero-tab-${String(activeTab)}`}
        className="p-5 sm:p-6"
        id={`hero-tabpanel-${String(activeTab)}`}
        role="tabpanel"
      >
        <AnimatePresence mode="wait">
          <motion.div
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -8 }}
            initial={{ opacity: 0, y: 8 }}
            key={activeTab}
            transition={SPRING_SNAPPY}
          >
            <CodeSnippet tokens={currentTokens} />
          </motion.div>
        </AnimatePresence>
      </div>
    </div>
  );
};

export default HeroCodeTabs;
