import Shell from "@/components/layout/shell.tsx";

const LOGOS = [
  { name: "TechCrunch", width: 120 },
  { name: "Product Hunt", width: 110 },
  { name: "Indie Hackers", width: 100 },
  { name: "Hacker News", width: 90 },
  { name: "The Verge", width: 115 },
  { name: "Wired", width: 105 },
] as const;

const LogoWall = () => {
  return (
    <section className="border-border border-y">
      <Shell variant="wide">
        <div className="flex flex-col items-center gap-8 py-12 sm:py-16">
          <p className="text-center text-muted-foreground text-sm tracking-wide">
            Trusted by writers worldwide
          </p>

          <div className="grid w-full grid-cols-2 divide-x divide-border sm:grid-cols-3 lg:grid-cols-6">
            {LOGOS.map((logo) => (
              <div
                className="logo-grayscale flex h-16 items-center justify-center px-6"
                key={logo.name}
              >
                <div className="flex items-center justify-center">
                  <span className="font-medium text-muted-foreground text-sm">
                    {logo.name}
                  </span>
                </div>
              </div>
            ))}
          </div>
        </div>
      </Shell>
    </section>
  );
};

export default LogoWall;
