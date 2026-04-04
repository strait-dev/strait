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

type SpendingLimitWarningProps = {
  name: string;
  planName: string;
  currentSpend: string;
  spendingLimit: string;
  percentUsed: string;
};

const SpendingLimitWarning = ({
  name,
  planName,
  currentSpend,
  spendingLimit,
  percentUsed,
}: SpendingLimitWarningProps) => (
  <Html>
    <Preview>You're approaching your spending limit</Preview>
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
            Spending limit warning
          </Heading>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            {name ? `Hello ${name},` : "Hello,"} your {planName} plan has used{" "}
            <strong style={{ color: "#252525" }}>{percentUsed}</strong> of your{" "}
            {spendingLimit} spending limit ({currentSpend} spent this billing
            period).
          </Text>

          <br />

          <Section>
            <Button
              className="inline-flex h-10 items-center justify-center rounded-[0.3rem] bg-[#171717] px-6 font-medium text-sm text-white no-underline transition-colors"
              href="https://app.usestrait.com/app/settings/billing"
            >
              Adjust spending limit
            </Button>
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
              &copy; 2026 Strait, All rights reserved
            </Text>
          </Section>
        </Container>
      </Body>
    </Tailwind>
  </Html>
);

SpendingLimitWarning.PreviewProps = {
  name: "Leonardo Santos",
  planName: "Pro",
  currentSpend: "$42.50",
  spendingLimit: "$50.00",
  percentUsed: "85%",
};

export default SpendingLimitWarning;
