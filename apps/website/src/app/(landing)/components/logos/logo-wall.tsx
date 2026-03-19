import Shell from "@/components/layout/shell.tsx";

const TECH_LOGOS = [
  {
    name: "Go",
    svg: (
      <svg className="h-6 w-auto" fill="currentColor" viewBox="0 0 120 50">
        <text dominantBaseline="central" fontSize="22" fontWeight="700" y="25">
          Go
        </text>
      </svg>
    ),
  },
  {
    name: "PostgreSQL",
    svg: (
      <svg className="h-6 w-auto" fill="currentColor" viewBox="0 0 120 50">
        <text dominantBaseline="central" fontSize="16" fontWeight="600" y="25">
          PostgreSQL
        </text>
      </svg>
    ),
  },
  {
    name: "Redis",
    svg: (
      <svg className="h-6 w-auto" fill="currentColor" viewBox="0 0 120 50">
        <text dominantBaseline="central" fontSize="18" fontWeight="600" y="25">
          Redis
        </text>
      </svg>
    ),
  },
  {
    name: "Fly.io",
    svg: (
      <svg className="h-6 w-auto" fill="currentColor" viewBox="0 0 120 50">
        <text dominantBaseline="central" fontSize="18" fontWeight="600" y="25">
          Fly.io
        </text>
      </svg>
    ),
  },
  {
    name: "React",
    svg: (
      <svg className="h-6 w-auto" fill="currentColor" viewBox="0 0 120 50">
        <text dominantBaseline="central" fontSize="18" fontWeight="600" y="25">
          React
        </text>
      </svg>
    ),
  },
] as const;

const LogoWall = () => {
  return (
    <section className="border-border border-y">
      <Shell variant="wide">
        <div className="flex flex-col items-center gap-8 py-12 sm:py-16">
          <p className="text-center text-muted-foreground/60 text-sm uppercase tracking-wider">
            Built with
          </p>

          <div className="grid w-full grid-cols-2 divide-x divide-border sm:grid-cols-3 lg:grid-cols-5">
            {TECH_LOGOS.map((logo) => (
              <div
                className="flex h-16 items-center justify-center px-6 text-muted-foreground/40"
                key={logo.name}
              >
                {logo.svg}
              </div>
            ))}
          </div>
        </div>
      </Shell>
    </section>
  );
};

export default LogoWall;
