import {
  Body,
  Button,
  Container,
  Font,
  Head,
  Heading,
  Hr,
  Html,
  Img,
  Link,
  Preview,
  Section,
  Tailwind,
  Text,
} from "@react-email/components";

type PaidPlanWelcomeProps = {
  name: string;
  planName: string;
  monthlyRunAllowance: string;
};

const PaidPlanWelcome = ({
  name,
  planName,
  monthlyRunAllowance,
}: PaidPlanWelcomeProps) => (
  <Html>
    <Preview>Welcome to Strait {planName}!</Preview>
    <Tailwind>
      <Head>
        <Font
          fallbackFontFamily="Helvetica"
          fontFamily="Geist"
          fontStyle="normal"
          fontWeight={400}
          webFont={{
            url: "https://fonts.googleapis.com/css2?family=Geist:wght@100..900&family=Geist:ital,opsz@0,14..32;1,14..32&display=swap",
            format: "woff2",
          }}
        />
        <Font
          fallbackFontFamily="Helvetica"
          fontFamily="Geist"
          fontStyle="normal"
          fontWeight={500}
          webFont={{
            url: "https://fonts.gstatic.com/s/Geist/v12/UcCO3FwrK3iLTeHuS_fvQtMwCp50KnMw2boKoduKmMEVuI6fAZ9hiA.woff2",
            format: "woff2",
          }}
        />
      </Head>
      <Body className="mx-auto my-auto bg-[#FFFFFF] font-sans">
        <Container className="mx-auto my-10 max-w-[500px] rounded-[0.1rem] border border-[#EBEBEB] border-solid px-10 py-8">
          <Section>
            <Img
              alt="Strait"
              className=""
              src="/static/strait-logo-black.svg"
              width="150"
            />
          </Section>

          <br />

          <Heading className="m-0 p-0 text-left font-semibold text-[#252525] text-lg tracking-tight">
            Welcome to Strait {planName}!
          </Heading>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            {name ? `Hello ${name},` : "Hello,"} thank you for upgrading to the{" "}
            {planName} plan.
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            Your plan includes{" "}
            <strong style={{ color: "#252525" }}>{monthlyRunAllowance}</strong>{" "}
            orchestration runs per month. To control overage beyond your
            included allowance, we recommend setting a spending cap:
          </Text>

          <br />

          <Section>
            <Button
              className="inline-flex h-10 items-center justify-center rounded-[0.3rem] bg-[#171717] px-6 font-medium text-sm text-white no-underline transition-colors"
              href="https://app.usestrait.com/app/settings/billing"
            >
              Set spending limit
            </Button>
          </Section>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            Here are some things you can do now:
          </Text>

          <br />

          <Section>
            <Text className="m-0 text-[#8D8D8D] text-sm leading-6">
              • View your{" "}
              <Link
                href="https://app.usestrait.com/app/settings/billing"
                style={{ color: "#171717", textDecoration: "underline" }}
              >
                billing dashboard
              </Link>
              <br />• Explore your{" "}
              <Link
                href="https://app.usestrait.com/app/workflows"
                style={{ color: "#171717", textDecoration: "underline" }}
              >
                workflows
              </Link>
              <br />• Monitor your{" "}
              <Link
                href="https://app.usestrait.com/app/runs"
                style={{ color: "#171717", textDecoration: "underline" }}
              >
                runs and events
              </Link>
            </Text>
          </Section>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-[12px] leading-6">
            If you have any questions, just reply to this email or contact our
            support team at{" "}
            <Link
              href="mailto:support@strait.dev"
              style={{ color: "#171717", textDecoration: "underline" }}
            >
              support@strait.dev
            </Link>
            .
          </Text>

          <br />

          <Hr className="mx-0 w-full border-[#EBEBEB] border-t" />

          <br />

          <Section>
            <Text className="m-0 text-left text-[#8D8D8D] text-[12px] leading-6">
              © 2026 Strait, All rights reserved
            </Text>
          </Section>
        </Container>
      </Body>
    </Tailwind>
  </Html>
);

PaidPlanWelcome.PreviewProps = {
  name: "Leonardo Santos",
  planName: "Pro",
  monthlyRunAllowance: "1,000,000",
};

export default PaidPlanWelcome;
