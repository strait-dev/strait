# CLAUDE.md

Read and follow `AGENTS.md` in this repository root — it is the primary operating guide.

## Git rules (non-negotiable)

- Always use conventional commit messages: `type(scope): summary`
- Never skip lefthook hooks. Never use `--no-verify`. If a hook fails, fix the issue.
- Never add "Co-Authored-By" lines to commit messages
- Never add "Generated with Claude Code" or any AI attribution to commits or PR descriptions
- Write helpful, substantive PR descriptions about what was actually worked on

## Project quick reference

- **Language**: Go 1.26, module `strait`
- **Main app**: `apps/strait/`
- **Build**: `cd apps/strait && go build ./...`
- **Test**: `cd apps/strait && go test ./...`
- **Lint**: `cd apps/strait && golangci-lint run --timeout=5m ./...`
- **Dependencies**: PostgreSQL, Redis (see `docker-compose.yml`)
- **Env vars**: see `.env.example` and `apps/strait/internal/config/config.go`
- **Migrations**: `apps/strait/migrations/` (embedded, auto-applied on startup)
- **Doppler**: `doppler secrets --project strait --config <dev|stg|prd>`
- **Fly**: apps `strait` and `strait-sequin`

## Key conventions

- Raw SQL with `pgx/v5` — no ORM
- Structured concurrency with `sourcegraph/conc`
- Error wrapping with `%w` and context
- Use `apps/strait/internal/testutil` helpers in tests
- No emojis in code, comments, logs, docs, or commits

## Design Context

### Users
Engineering teams, AI builders, and DevOps/SREs who build and manage background job infrastructure. They arrive frustrated with fragmented tooling -- multiple queues, custom retry logic, shell-script workflows -- and need a production-grade platform they can trust immediately. They evaluate tools by reading code examples, scanning feature comparisons, and testing interactively. They value clarity over flash.

### Brand Personality
**Precise. Reliable. Sharp.**

Strait is an engineered tool, not a decorated one. Every design decision should feel intentional and earned. The brand communicates competence through restraint -- clean typography, purposeful color, and interfaces that look like they were built by the same engineers who built the product.

### Emotional Goals
- **Confidence**: This is production-grade. I can trust my infrastructure to this.
- **Excitement**: This is well-built technology. I want to try it right now.

### Aesthetic Direction
- **References**: Linear.app (minimal, fast, precise dark mode with restrained color), Vercel.com (clean typography, strong hierarchy, product-focused interactive demos)
- **Anti-references**: Flashy SaaS (no gradient explosions, no 3D illustrations, no floating abstract shapes), enterprise boring (no stock photos, no blue corporate, no walls of text), over-animated showcases (motion serves understanding, not spectacle)
- **Theme**: Dark mode primary. Teal accent (configurable via data-accent). Geist Sans typography.
- **Color**: OKLch color space. One accent color per view. Tinted neutrals toward the brand hue. Never pure black or pure white.

### Design Principles
1. **Clarity over decoration** -- Every element must earn its place. If it doesn't help the user understand the product or take action, remove it.
2. **One accent, used boldly** -- The teal/primary accent appears sparingly but with purpose: highlighted keywords, active states, primary CTAs. Never sprinkled at low opacity everywhere.
3. **Show the product** -- Interactive demos, real code, and concrete feature comparisons beat abstract illustrations and marketing copy. Let the product sell itself.
4. **Rhythm through contrast** -- Vary section backgrounds, spacing, and density to create visual rhythm. Alternating light/dark sections, thin dividers, and varied layouts prevent scroll fatigue.
5. **Motion with purpose** -- Animate only to convey state changes, guide attention, or reveal content progressively. Never exceed 200ms for interaction feedback. Respect prefers-reduced-motion.
