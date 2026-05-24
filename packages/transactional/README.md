# @strait/transactional

Transactional email templates built with React Email. Contains all email templates sent by the Strait platform, organized by domain.

## Templates

### Auth (6)
ChangeEmail, ConfirmAccount, DeleteAccount, MagicLink, PasswordUpdate, ResetPassword

### Billing (5)
OverageAlert, PaidPlanWelcome, PaymentFailed, PlanChanged, SpendingLimitWarning

### Organization (4)
OrganizationDeleted, OrganizationInvite, OrganizationPurged, OrganizationVerificationCode

### Common (5)
Contact, Feedback, Goodbye, Support, Welcome

## Usage

All templates are re-exported from the package root:

```ts
import { MagicLink, OrganizationInvite, Feedback } from "@strait/transactional";
```

## Used by

- `apps/app` -- sends emails via `auth.server.ts`, `organization-handler.ts`, feedback/support dialogs

## Development

```sh
bun run dev          # launch React Email preview on port 3001
bun run export       # export templates to HTML
bun run typecheck    # type-check with tsgo
```
