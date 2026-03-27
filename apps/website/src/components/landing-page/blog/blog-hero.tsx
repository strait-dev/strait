import { basehub } from "basehub";

import Shell from "@/components/layout/shell.tsx";

type HeroData = {
  _title?: string;
  badge?: string;
  subtitle?: string;
};

const BlogHero = async () => {
  const query = {
    website: {
      blog: {
        hero: {
          _title: true,
          badge: true,
          subtitle: true,
        },
      },
    },
  };

  const data = (await basehub({ draft: false }).query(query as never)) as {
    website: { blog: { hero?: HeroData } };
  };

  const heroData = data.website.blog.hero;
  const badge = heroData?.badge as string | undefined;
  const title = heroData?._title as string | undefined;
  const subtitle = heroData?.subtitle as string | undefined;

  const hasRequiredContent = badge && title && subtitle;

  if (!hasRequiredContent) {
    return null;
  }

  return (
    <section className="relative isolate overflow-hidden pt-32 pb-12 sm:pt-40 sm:pb-16">
      <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_var(--primary)/0.06,_transparent_40%)]" />
      <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_transparent,_var(--background)_70%)]" />
      <div className="paper-texture absolute inset-0 -z-10 opacity-[0.02]" />

      <Shell variant="wide">
        <div className="mx-auto flex max-w-3xl flex-col items-center text-center">
          <span className="kicker">{badge}</span>

          <h1 className="mt-6 text-balance text-4xl leading-[1.15] sm:text-5xl lg:text-6xl">
            <span className="text-foreground">{title}</span>{" "}
            <span className="text-muted-foreground">{subtitle}</span>
          </h1>
        </div>
      </Shell>
    </section>
  );
};

export default BlogHero;
