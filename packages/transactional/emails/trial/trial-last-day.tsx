import {
  Body,
  Button,
  Container,
  Font,
  Head,
  Hr,
  Html,
  Img,
  Link,
  Preview,
  Section,
  Tailwind,
  Text,
} from "@react-email/components";

type Props = {
  name: string;
  upgradeUrl: string;
};

const TrialLastDay = ({
  name = "Leonardo",
  upgradeUrl = "https://app.usestrait.com/upgrade?discount=20",
}: Props) => {
  const previewText = "Don't lose what you've already built on Strait";

  return (
    <Html>
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
      <Preview>{previewText}</Preview>
      <Tailwind>
        <Body className="mx-auto my-auto bg-[#FFFFFF] font-sans">
          <Container className="mx-auto my-10 max-w-[500px] rounded-[0.1rem] border border-[#EBEBEB] border-solid px-10 py-8">
            <Section>
              <Img
                alt="Strait Logo"
                className=""
                src="/static/strait-logo-black.svg"
                width="150"
              />
            </Section>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              Hello {name},
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              Your Strait trial period ends{" "}
              <span className="font-medium">today</span>. We don't want you to
              lose all the progress you've already made on our sales management
              platform.
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              <span className="font-medium">Special last-minute offer:</span> To
              make your decision easier, we're offering 20% off the first month
              of any plan if you upgrade today. This offer expires at midnight.
            </Text>

            <br />

            <Section>
              <Button
                className="inline-flex h-10 items-center justify-center rounded-[0.3rem] bg-[#171717] px-6 font-medium text-sm text-[#FFFFE3] no-underline transition-colors"
                href={upgradeUrl}
              >
                Upgrade with 20% discount
              </Button>
            </Section>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              If you prefer, we can schedule a quick conversation to discuss
              your specific needs and ensure you choose the most suitable plan.
              Just reply to this email or schedule through{" "}
              <Link
                className="text-[#171717] underline"
                href="https://calendly.com/strait/15min"
                style={{ color: "#171717", textDecoration: "underline" }}
              >
                our calendar
              </Link>
              .
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              Best regards,
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              Leo
              <br />
              Founder of Strait
            </Text>

            <br />

            <Hr className="mx-0 w-full border-[#EBEBEB] border-t" />

            <br />

            <Section>
              <Text className="m-0 text-left text-[#8D8D8D] text-[12px] leading-6">
                © 2025 Strait, All rights reserved
              </Text>
            </Section>
          </Container>
        </Body>
      </Tailwind>
    </Html>
  );
};

TrialLastDay.PreviewProps = {
  name: "Leonardo",
  upgradeUrl: "https://app.usestrait.com/upgrade?discount=20",
};

export default TrialLastDay;
