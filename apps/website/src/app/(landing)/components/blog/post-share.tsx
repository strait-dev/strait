"use client";

import {
  Copy01Icon,
  Share01Icon,
  Tick01Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { useCallback, useState } from "react";

const BASE_URL = process.env.NEXT_PUBLIC_WEBSITE_URL || "https://trystrait.ai";

type PostShareProps = {
  title: string;
  slug: string;
};

const PostShare = ({ title, slug }: PostShareProps) => {
  const [copied, setCopied] = useState(false);
  const shareUrl = `${BASE_URL}/blog/${slug}`;

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(shareUrl);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      console.error("Failed to copy link");
    }
  }, [shareUrl]);

  const handleShare = useCallback(async () => {
    if (navigator.share) {
      try {
        await navigator.share({
          title,
          url: shareUrl,
        });
      } catch {
        handleCopy();
      }
    } else {
      handleCopy();
    }
  }, [title, shareUrl, handleCopy]);

  return (
    <div className="flex items-center gap-2">
      <Button onClick={handleShare} size="default" variant="outline">
        <HugeiconsIcon className="size-4" icon={Share01Icon} />
        <span>Share</span>
      </Button>

      <Button onClick={handleCopy} size="default" variant="ghost">
        <HugeiconsIcon
          className="size-4"
          icon={copied ? Tick01Icon : Copy01Icon}
        />
        <span>{copied ? "Copied!" : "Copy link"}</span>
      </Button>
    </div>
  );
};

export default PostShare;
