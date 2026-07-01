# @strait/transactional

Transactional email templates built with React Email. Contains all email templates sent by the Strait platform, organized by domain.

## Source Of Truth

Templates live under `emails/` and are exported from the package root. Keep product copy in the templates themselves; keep operational email policy in customer or support documentation.

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

## Used by

- `apps/app` -- sends emails via `auth.server.ts`, `organization-handler.ts`, feedback/support dialogs

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
```
