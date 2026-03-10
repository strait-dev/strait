"use client";

import { Copy01Icon, Tick01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { toast } from "@strait/ui/toast";
import copy from "copy-to-clipboard";
import { AnimatePresence, motion } from "motion/react";
import { useState } from "react";
import { Button } from "./button";
import { Tooltip, TooltipContent, TooltipTrigger } from "./tooltip";

interface IdCellProps {
  id: string;
}

export function IdCell({ id }: IdCellProps) {
  const [isCopied, setIsCopied] = useState(false);
  const truncatedId = id.slice(0, 6);

  const handleCopy = () => {
    copy(id);
    setIsCopied(true);
    toast.success("ID copied to clipboard");
    setTimeout(() => {
      setIsCopied(false);
    }, 2000);
  };

  const Icon = isCopied ? Tick01Icon : Copy01Icon;

  return (
    <div className="flex items-center gap-2">
      <Tooltip>
        <TooltipTrigger
          render={<span className="font-mono text-muted-foreground text-sm" />}
        >
          {truncatedId}...
        </TooltipTrigger>
        <TooltipContent>
          <span className="font-mono">{id}</span>
        </TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              className="size-7"
              onClick={handleCopy}
              size="icon"
              variant="ghost"
            />
          }
        >
          <AnimatePresence initial={false} mode="wait">
            <motion.span
              animate={{ scale: 1, opacity: 1 }}
              className="flex items-center justify-center"
              exit={{ scale: 0, opacity: 0 }}
              initial={{ scale: 0, opacity: 0 }}
              key={isCopied ? "check" : "copy"}
              transition={{ duration: 0.15 }}
            >
              <HugeiconsIcon icon={Icon} size={12} />
            </motion.span>
          </AnimatePresence>
        </TooltipTrigger>
        <TooltipContent>Copy ID</TooltipContent>
      </Tooltip>
    </div>
  );
}
