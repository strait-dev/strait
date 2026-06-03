import { Button } from "@strait/ui/components/button";
import { Field, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";

type SsoFormProps = {
  redirectTo?: string;
};

// SSO is temporarily disabled due to an upstream dependency issue
// in @better-auth/sso (samlify CJS/ESM incompatibility).
// Tracking: https://github.com/better-auth/better-auth/issues/8620
const SsoForm = (_props: SsoFormProps) => (
  <form
    onSubmit={(e) => {
      e.preventDefault();
    }}
  >
    <div className="flex flex-col gap-4">
      <Field className="w-full">
        <FieldLabel htmlFor="sso-email">Work email</FieldLabel>
        <Input
          autoComplete="email"
          disabled
          id="sso-email"
          placeholder="you@company.com"
          type="email"
        />
      </Field>

      <Button
        className="w-full"
        disabled={true}
        type="submit"
        variant="brand-solid"
      >
        SSO coming soon
      </Button>

      <p className="text-center text-muted-foreground text-xs">
        Enterprise SSO is temporarily unavailable. Please use another sign-in
        method.
      </p>
    </div>
  </form>
);

export default SsoForm;
