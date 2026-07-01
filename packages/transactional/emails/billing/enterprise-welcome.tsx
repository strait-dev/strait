import { Link, Section, Text } from "@react-email/components";
import {
  appBillingUrl,
  BillingButton,
  BillingLayout,
  BillingText,
  salesEmail,
} from "./billing-layout";

const EnterpriseWelcome = () => (
  <BillingLayout
    heading="Welcome to Strait Enterprise!"
    preview="Welcome to Strait Enterprise"
  >
    <BillingText>
      Welcome to Strait Enterprise. Your dedicated Customer Success Manager will
      reach out within 1 business day to schedule your onboarding session.
    </BillingText>

    <br />

    <BillingText>Your Enterprise launch plan includes:</BillingText>

    <br />

    <Section>
      <Text className="m-0 text-[#8D8D8D] text-sm leading-7">
        •{" "}
        <strong style={{ color: "#252525" }}>
          Custom orchestration run allowance
        </strong>{" "}
        and overage terms from your contract
        <br />•{" "}
        <strong style={{ color: "#252525" }}>
          Custom concurrency, step caps, and history retention
        </strong>
        <br />•{" "}
        <strong style={{ color: "#252525" }}>Consolidated invoicing</strong> for
        contracted organizations
        <br />•{" "}
        <strong style={{ color: "#252525" }}>
          SLA target and support terms
        </strong>{" "}
        from your contract
        <br />•{" "}
        <strong style={{ color: "#252525" }}>Dedicated Slack channel</strong>{" "}
        for direct engineering support
      </Text>
    </Section>

    <br />

    <BillingText>
      Roadmap and contact-sales items such as SSO/SAML, SCIM, static IPs, VPC
      peering, data residency, single-tenant orchestration, and BYO-cloud are
      not launch entitlements unless they are explicitly committed in your
      contract.
    </BillingText>

    <br />

    <BillingButton href={appBillingUrl}>View billing dashboard</BillingButton>

    <br />

    <BillingText>
      If you have any questions before your onboarding session, reply to this
      email or contact us at{" "}
      <Link
        href={`mailto:${salesEmail}`}
        style={{ color: "#171717", textDecoration: "underline" }}
      >
        {salesEmail}
      </Link>
      .
    </BillingText>
  </BillingLayout>
);

EnterpriseWelcome.PreviewProps = {};

export default EnterpriseWelcome;
