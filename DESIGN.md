# Strait Design Context

Design and brand reference for the app, docs, and marketing site: voice, visual
direction, and the shared design-system tokens. This is not a product overview;
for what Strait does, see [`README.md`](README.md) and
[`apps/docs/introduction.mdx`](apps/docs/introduction.mdx).

## Product

Strait is a job execution and workflow orchestration platform. It runs background jobs, scheduled tasks, and multi-step workflows. Ships as a single Go binary backed by PostgreSQL and Redis. Two editions: open-source community (self-hosted) and cloud (SaaS at strait.dev).

## Audience

Full-stack developers who need reliable job scheduling without managing infrastructure. They evaluate tools quickly, care about developer experience, and expect production-grade reliability from day one. They read docs, try the quickstart, and judge the product in the first session.

## Brand voice

Technical, confident, calm.

Strait speaks with authority but never condescends. It assumes you're competent. Copy is direct and precise: no marketing fluff, no exclamation marks, no empty promises. When something is complex, it explains clearly. When something is simple, it stays out of the way.

## Interface feel

Clean and focused, with calm surfaces and depth on demand. The UI should feel like a well-maintained control room: quiet when everything is running, informative when you need to investigate, and fast to navigate. Information density is earned through hierarchy, not clutter.

## Visual references

- Linear: minimal chrome, purposeful use of color, information-dense without feeling crowded
- Vercel: dark-first, precise spacing, typographic confidence
- Resend: warm accents on dark backgrounds, developer-focused but not cold

## Anti-references

- Generic admin templates with no personality or craft
- Enterprise dashboards drowning in blue gradients and tiny illegible text
- Consumer apps with rounded everything, pastel colors, and playful illustrations
- Template landing pages with purple-cyan gradients and "hero metric" layouts

## Design system

- **Primary color**: oklch(0.6696 0.222 37.42), a warm orange/red used for primary actions, accents, and brand identity
- **Dark mode default**: Both the app and website default to dark mode
- **Typography**: Geist (sans) + Geist Mono (code). No other fonts.
- **Radius**: 0.45rem (compact, not rounded)
- **Base color**: Neutral (pure neutral, not zinc or slate)
- **Component library**: shadcn/ui (base-nova style, Base UI primitives)
- **Icon library**: Hugeicons
- **Chart palette**: Blue-tinted (oklch 251-265 hue range)

## Typography scale (app)

- h1: text-xl font-normal tracking-tight
- h2: text-lg font-normal
- h3: text-sm font-medium
- h4: text-xs font-medium uppercase tracking-wider

## Typography scale (website)

The marketing site lives in its own repo: <https://github.com/strait-dev/website>. The scale below is mirrored here so the app, docs, and marketing site stay visually consistent.

- h1: text-4xl sm:text-5xl lg:text-6xl font-bold tracking-tight
- h2: text-2xl sm:text-3xl lg:text-4xl font-bold tracking-tight
- h3: text-xl sm:text-2xl font-semibold tracking-tight

## Button sizing

Default size everywhere. No size="sm" or size="lg" on text buttons. Icon-only buttons use size="icon" variants.
