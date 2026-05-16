---
target: strait marketing website
total_score: 24
p0_count: 1
p1_count: 2
timestamp: 2026-05-15T15-37-54Z
slug: strait-website
---
# Critique: Strait marketing website

**Targets**: `strait-dev/website` repo, cloned at `/Users/leonardomaldonado/conductor/workspaces/strait/_critique/website/apps/website`. Astro + Tailwind, ~16 pages, 102 markup files.

## Design Health Score

| # | Heuristic | Score | Key Issue |
|---|-----------|-------|-----------|
| 1 | Visibility of System Status | 3/4 | DAG/pipeline demos show live state; CTA hovers give no feedback beyond shadow |
| 2 | Match System / Real World | 3/4 | Copy is precise; kicker labels are generic uppercase |
| 3 | User Control and Freedom | 2/4 | No back-to-top, no sticky ToC on feature/compare; only exit is browser back |
| 4 | Consistency and Standards | 3/4 | Inconsistent heading weight on feature pages (h2 missing weight class) |
| 5 | Error Prevention | 2/4 | Blog silently renders "No posts found. Check back soon!" when CMS fails |
| 6 | Recognition Rather Than Recall | 2/4 | Nav mega-menu groups require recall of Strait taxonomy |
| 7 | Flexibility and Efficiency | 2/4 | No keyboard layer; date range picker disabled with no explanation |
| 8 | Aesthetic and Minimalist | 2/4 | Every section is heading + sub + body. Six bento cards identical anatomy. Rhythm never breaks. |
| 9 | Error Recovery | 2/4 | 404 generic, no search/sitemap/did-you-mean; privacy is placeholder-quality |
| 10 | Help and Documentation | 3/4 | Footer link to docs; copy-able code examples; no inline doc links on feature specs |
| **Total** | | **24/40** | **Below average for a modern marketing site** |

## Anti-Patterns Verdict

**LLM verdict: Leaning yes on AI slop.** The site shows hallmarks of AI-scaffolded composition: an implausibly complete animation utility set (`parallax-slow`, `hero-tl`, `hero-draft-card`, `demo-chat-msg`, `why-card-el`, `fsm-dot`) where many are unused on actual pages — generated as a block, not grown alongside usage. The landing sequence (hero kicker → h1 with primary-color span → two-button CTA → bento grid → code tabs → comparison table → pricing → FAQ) maps exactly to the dev-tool landing template LLMs default to. Component names like `HeroDag`, `CredibilitySection`, `ArchitectureList` are functional labeling.

**Category-reflex first-order: FAIL.** Knowing only "dev tool" you'd predict exactly here: dark mode, single warm accent, neutral grays, Geist. No alternatives were considered.

**Category-reflex second-order: FAIL.** Given the anti-references rule out blue-gradient SaaS and pastel-consumer, the natural answer would be terminal-native or editorial-technical. Site delivers shadcn-template neutral with bento — the middle ground anti-refs were meant to exclude.

**Deterministic detector:** 1 finding only (`overused-font` for Geist in `fonts.css`). No gradient-text, no glassmorphism, no card-on-card, no drop-shadow-everywhere. The detector confirms: visible problems are composition decisions, not rule violations.

## Cognitive Load

**5 of 8 checklist items fail:**
1. Hero CTAs: 2 options ✓
2. Body line length 65–75ch: bento card bodies in wide spans run to 90ch+ ✗
3. Hierarchy ratio: h2 weight collapses to inconsistent classes ✗ (borderline)
4. Motion animates only opacity/transform: `SPRING_BOUNCY` (stiffness 400, damping 15) produces overshoot ✗
5. Primary color as signal not decoration: orange appears 7 times across one scroll (CTA, h1 word, bento dot, hover, table column, badge, architecture) — signal diluted ✗
6. No nested cards: use-case page stacks bordered cards inside bordered sections ✗
7. Theme scene-driven not category-reflex: `html.dark` hardcoded, no light mode, no scene articulated ✗
8. No gradient text ✓

## What's Working

1. **The DAG animation in the bento grid** (`apps/website/src/components/landing/hero-dag.tsx`) — uses CSS custom props, respects `prefers-reduced-motion`, runs on rAF with IntersectionObserver pause, pedagogically accurate fan-out → retry → completion. The one piece of UI that shows the product, not a category.
2. **Code example section** (`apps/website/src/components/landing-page/code-examples/code-example-section.tsx`) — tabbed code with arrow-key nav, copy with timeout reset, snappy spring (not bouncy), wrapped in MockBrowserWindow. Four examples tell a coherent product story.
3. **The `infinity-border-y` system** (`globals.css:524-570`) — full-bleed pseudo-element borders that break card-grid monotony by connecting sections horizontally. Gestures toward the "control room" brief.

## Priority Issues

**[P0] Dark neutrals are pure chroma-0 gray; brand hue is never absorbed into surfaces.**
`--background: oklch(0.145 0 0)`, `--card: oklch(0.205 0 0)`, `--muted: oklch(0.269 0 0)`. Every surface is zero-hue. The brand orange (hue ~37) reads as a foreign object pasted on top, not as light coming from the product. This is the single largest reason the site feels like "shadcn template with a color change." Linear, Vercel, Resend all warm/tint their dark neutrals toward the brand hue.
Fix: `--background: oklch(0.145 0.004 37)`, `--card: oklch(0.205 0.006 37)`, `--muted: oklch(0.269 0.005 37)`. Invisible as color, but surfaces stop fighting the brand.
Command: `$impeccable colorize`

**[P1] `SPRING_BOUNCY` on scroll reveals violates the motion law.**
`apps/website/src/lib/motion.ts:16` — `stiffness: 400, damping: 15`. Underdamped, visible overshoot. Used in `Reveal` for bento cards. Every feature card bounces on scroll-in. Collides with the "calm control room" brief.
Fix: Rename to `SPRING_FAST` with `stiffness: 280, damping: 26` (critically damped), or drop spring and use `duration: 0.4, ease: [0.22, 1, 0.36, 1]`.
Command: `$impeccable animate`

**[P1] Every section follows heading + sub + body with no structural variation.**
ProblemSection, FeatureBentoGrid, CodeExampleSection, CredibilitySection, ComparisonSection, PricingTeaser, all inner-page section groups — same shape, `py-16 sm:py-20` to `py-20 sm:py-28`. Rhythm requires variation; same shape everywhere is repetition, not rhythm.
Fix: At least two sections per page break the pattern: editorial column pull (huge heading, narrow body), no-heading section leading with a large UI element, full-bleed with text right-aligned against left illustration.
Command: `$impeccable layout`

**[P2] Primary CTA uses `bg-gradient-to-r from-primary to-primary/80` everywhere.**
`Header.astro:50`, `CTA.astro:44`, `features/[slug].astro:73`, `compare/[slug].astro:72`, `use-cases/[slug].astro:73`. A gradient from primary to primary/80 is the minimum meaningful gradient — primary with a directional fade to 80% opacity. Registers as failed depth. Pattern from template sites; Vercel/Linear use flat brand fills.
Fix: `bg-primary hover:bg-primary/90` or `hover:brightness-110`. Or commit: hue-shifted gradient that tells a story.
Command: `$impeccable bolder`

**[P2] Kicker is muted-gray uppercase — no brand anchor.**
`.kicker` in `globals.css:422`: same treatment as every other label. First typographic element a visitor reads on most pages.
Fix: Branded chip with `bg-primary/10 border-primary/30 text-primary`, or thin primary line before label.
Command: `$impeccable typeset`

**[P2] Hero has no product proof above the fold.**
`index.astro:73-112` ends at the two CTA buttons — pure copy + faint grid bg. `HeroProductPreview` was built (`hero.tsx`, complete component with animated tabs) and NOT used. Decision made and reversed; component left in repo.
Fix: Adopt `HeroProductPreview` in the index hero, or promote the bento DAG animation into the hero.
Command: `$impeccable onboard`

**[P3] `ComparisonSection.astro` table is self-serving.**
Strait column gets checkmark + bold for every row. Strait cells read as marketing copy ("Built-in per-run budgets"), competitor cells read as raw values (`wait.forToken()`, "Via Signals"). Senior engineers spot this format immediately.
Fix: Normalize — all rows checkmarks OR all rows raw values. Remove checkmark from Strait column; highlight only column header with `bg-primary/5`.
Command: `$impeccable distill`

**[P3] `hover-lift` uses `ease` timing function; should be ease-out-expo.**
`globals.css:444-456` — `transform: translateY(-4px)` + `ease`. Brief calls for ease-out-quart/quint/expo only.
Fix: `cubic-bezier(0.22, 1, 0.36, 1)`. Clamp translateY to `-2px`.
Command: `$impeccable polish`

## Persona Red Flags

**The Power Evaluator (HN, <45s to decide):** Lands on index. Hero is text + grid background + two buttons. No screenshot, no live number, no terminal output. Scrolls. Reaches bento → DAG animation — one full viewport-height past the hero. May never get there. Hero supporting copy (`text-sm leading-relaxed sm:text-base` at `mt-4`) feels like continuation of headline, not a breathing pause before CTAs. Cannot tell what makes Strait worth 45 more seconds.

**The Pragmatist (came from "Temporal vs X" search, on `/compare/temporal`):** Differentiator cards use the same `text-sm leading-relaxed` `text-muted-foreground/70` for both Strait and Temporal text. No visual hierarchy between "what Strait does" and "what Temporal does." Has to read every word, cannot scan. The feature comparison table is `client:only` — on a slow connection sees nothing in that section until hydration, no skeleton.

**The Skeptical Senior (pricing page):** Booleans are checkmarks. Wants to know what happens at the limit. "Unlimited" appears in Free and Enterprise columns with no asterisk, no fair-use link, no footnote. Enterprise has only `boolean` true for "Dedicated compute," "Static IPs," "VPC peering" — no pricing range, just "Contact us." Cannot scope the cost. Leaves.

## Minor Observations

- `footer.astro:29` links to `github.com/leonardomso/strait` (founder personal) not a `strait-dev` org URL.
- `section-muted` (`globals.css:495`) uses chroma-0 in both modes; missed first application of brand-tinted neutrals.
- `MeshGradientBg` component exists, isn't used anywhere — code debt.
- Blog renders "No posts found. Check back soon!" silently when CMS unavailable (`blog/index.astro:126`).
- `SPRING_BOUNCY` is named bouncy — author knew it would bounce. Should be `SPRING_SNAPPY` for card reveals.
- `privacy.astro` reads as placeholder content.
- Every page's bottom CTA is the same `CTA.astro` with the same backgrounds.
- Pricing toggle buttons (`rounded-full px-5 py-2.5`) larger than nav CTAs (`h-9`) — mixed sizing.
- `compare/[slug].astro:133-144`: `<li>` inside `<div>` without a surrounding `<ul>` — invalid HTML.

## Questions to Consider

1. **What is the physical scene?** Brief says "control room." Site dark mode has no warmth, texture, material quality — blank dark canvas. A control room has ambient illumination, bezels, status lights. What would it mean to actually make this feel like a control room, vs describe it as one?
2. **Why is the hero product-free?** `HeroProductPreview` was built (complete, 4-tab, animated, keyboard-accessible) and removed. Was that decision correct? Power evaluator has no visual evidence until they scroll past hero.
3. **What does the brand orange mean in dark context?** At 12% opacity tint, at 100% button, between those values it's a dot, table highlight, hover state, kicker color, accent word. A color that carries meaning, or a color that fills slots?
4. **Is "AI agents" the right h1 anchor?** "Background jobs, workflows, and AI agents that never lose state." AI agents is last in the list but gets primary-color treatment. Strait is opportunistically positioning, or AI-agent-platform-first? If opportunistic, color primacy on it misleads the background-job buyers who are the natural audience.
5. **What would a version without a single card look like?** Bento has 6 cards. Problem section has 4. Credibility = 3 columns acting as cards. Compare page has differentiator cards + feature highlight cards. Pricing has plan cards + comparison rows. If you removed every card and used only typography, whitespace, rules, and brand color — what would you discover about which content actually has hierarchy vs what's just being given equal weight by a container?
