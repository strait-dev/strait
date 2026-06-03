import { Button } from "@strait/ui/components/button";
import { Field, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";

type SsoFormProps = {
  redirectTo?: string;
};

// SSO is not a launch entitlement. Keep this route disabled until it moves
// from roadmap/contact-sales status to an enforced product capability.
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
        SSO roadmap
      </Button>

      <p className="text-center text-muted-foreground text-xs">
        SSO is not available in launch plans. Please use another sign-in method.
      </p>
    </div>
  </form>
);

export default SsoForm;
