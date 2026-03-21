import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { basehub } from "basehub";
import { draftMode } from "next/headers";
import Link from "next/link";

import { dashboardHref } from "@/lib/urls.ts";

type CtaData = {
  badge?: string;
  title?: string;
  description?: string;
  button_text?: string;
  button_href?: string;
  subtext?: string;
};

const PostCtaSidebar = async () => {
  const draft = await draftMode();

  const query = {
    website: {
      home: {
        cta: {
          cta: {
            badge: true,
            title: true,
            description: true,
            button_text: true,
            button_href: true,
            subtext: true,
          },
        },
      },
    },
  };

  const data = (await basehub({ draft: draft.isEnabled }).query(
    query as never
  )) as { website: { home: { cta?: { cta?: CtaData } } } };

  const ctaData = data.website.home.cta?.cta;
  const badge = ctaData?.badge as string | undefined;
  const title = ctaData?.title as string | undefined;
  const description = ctaData?.description as string | undefined;
  const buttonText = ctaData?.button_text as string | undefined;
  const buttonHref = ctaData?.button_href as string | undefined;

  const hasRequiredContent =
    badge && title && description && buttonText && buttonHref;

  if (!hasRequiredContent) {
    return null;
  }

  return (
    <div className="sticky top-24">
      <div className="rounded-2xl border border-border/60 bg-gradient-to-b from-muted/50 to-background p-6">
        <span className="inline-flex items-center rounded-full bg-muted px-3 py-1 font-medium text-foreground text-xs">
          {badge}
        </span>

        <h3 className="mt-4 text-foreground text-lg tracking-tight">{title}</h3>

        <p className="mt-2 text-muted-foreground text-sm leading-relaxed">
          {description}
        </p>

        <Button
          className="mt-6 w-full"
          render={<Link href={dashboardHref(buttonHref)} />}
          size="default"
        >
          {buttonText}
          <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
        </Button>
      </div>
    </div>
  );
};

export default PostCtaSidebar;
