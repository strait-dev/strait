# @strait/transactional

Transactional email templates built with React Email. Contains all email templates sent by the Strait platform, organized by domain.

## Source Of Truth

Templates live under `emails/` and are exported from the package root. Keep product copy in the templates themselves; keep operational email policy in customer or support documentation.

The registry in `emails/registry.tsx` is the source of truth for Go-triggered
email template IDs, subjects, React components, and strict prop schemas. The Go
service sends typed email intents to `apps/app`; `apps/app` validates the props,
renders the template, and sends the email with Resend.

## Templates

### Auth (6)
ChangeEmail, ConfirmAccount, DeleteAccount, MagicLink, PasswordUpdate, ResetPassword

### Billing (14)
ContractExpired, DisputeAlert, DowngradeHTTPJobsWarning, DunningStep, EnterpriseContractReminder, EnterpriseWelcome, InvoiceUpcoming, OverageAlert, PaidPlanWelcome, PaymentFailed, PlanChanged, SpendingLimitWarning, TrialEndingSoon, UsageReport

### Organization (4)
OrganizationDeleted, OrganizationInvite, OrganizationPurged, OrganizationVerificationCode

### Common (5)
Contact, Feedback, Goodbye, Support, Welcome

### Notifications (6)
BudgetThreshold, CostAnomaly, GenericNotification, NotificationSpendingLimitReached, NotificationSpendingLimitWarning, UsageForecastWarning

## Usage

All templates are re-exported from the package root:

```ts
import { MagicLink, OrganizationInvite, Feedback } from "@strait/transactional";
```

Resend-backed delivery helpers are exported from the same package:

```tsx
import {
  PaymentFailed,
  createTransactionalEmailSender,
} from "@strait/transactional";

const sender = createTransactionalEmailSender({
  apiKey: process.env.RESEND_API_KEY,
  from: "billing@strait.dev",
});

await sender.send({
  to: "admin@example.com",
  subject: "Action required: payment failed",
  react: (
    <PaymentFailed
      gracePeriodEnd="April 15, 2026"
      name="Leonardo"
      planName="Pro"
    />
  ),
});
```

Go-triggered emails do not import template components directly. The internal
`POST /internal/transactional-email` endpoint in `apps/app` resolves the
template id sent by the Go service, validates the untyped `props` payload
against that template's own schema, then renders and sends it:

```tsx
import { resolveTransactionalEmailTemplate } from "@strait/transactional/registry";

const template = resolveTransactionalEmailTemplate("billing.payment_failed");
if (!template) {
  throw new Error("unknown transactional email template");
}

const parsedProps = template.schema.safeParse({
  gracePeriodEnd: "April 15, 2026",
  name: "Leonardo",
  planName: "Pro",
});
if (!parsedProps.success) {
  throw new Error("invalid transactional email template props");
}

const subject = template.subject(parsedProps.data);
const element = template.render(parsedProps.data);
```

`transactionalEmailTemplateIds` (every valid template id) and the
`TransactionalEmailTemplateId` union type are exported from the same
`./registry` entry point for validating or typing template ids elsewhere.

## Used by

- `apps/app` -- sends app-owned emails via `auth.server.ts`,
  `organization-handler.ts`, feedback/support dialogs, and the internal
  `POST /internal/transactional-email` endpoint used by Go-triggered emails.
- `apps/strait` -- sends billing, notification, and monthly usage-report
  intents to `apps/app` using `APP_INTERNAL_URL`, `INTERNAL_SECRET`, and
  `TRANSACTIONAL_EMAIL_TIMEOUT`.

## Runtime Environment

Hosted environments keep these variables in Infisical:

| Service | Variables |
|---|---|
| `apps/strait` | `APP_INTERNAL_URL`, `INTERNAL_SECRET`, `TRANSACTIONAL_EMAIL_TIMEOUT`, `RESEND_FROM_EMAIL` |
| `apps/app` | `INTERNAL_SECRET`, `RESEND_API_KEY`, `RESEND_FROM_EMAIL`, `RESEND_SUPPORT_EMAIL` |

`INTERNAL_SECRET` must match across both services for the same environment. The
Go client forwards Resend idempotency keys to the app endpoint; no app or Go
email outbox table is used.

## Development

```bash
bun run dev          # launch React Email preview on port 3001
bun run export       # export templates to HTML
bun run typecheck    # type-check with tsgo
```

## Validation

```bash
bun run --cwd packages/transactional typecheck
bun run --cwd packages/transactional biome:lint
bun run --cwd packages/transactional test
```
