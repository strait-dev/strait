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
  trialEndDate: string;
  upgradeUrl: string;
};

const WelcomeTrial = ({
  name = "Leonardo",
  trialEndDate = "March 23, 2025",
  upgradeUrl = "https://app.usestrait.com/upgrade",
}: Props) => {
  const previewText = "Welcome to Strait! I'm here to help you grow";

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
                src="https://mwesulbn1k.ufs.sh/f/DedoMBfQiCy9vOEDu2YCvLugTtO8VEnoywN2DbkUr6QB1MP3"
                width="150"
              />
            </Section>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              Hello {name},
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              I'm Leo, founder of Strait, and I want to welcome you! I'm very
              happy that you decided to try our sales management platform.
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              I created Strait because I noticed that many entrepreneurs like
              you faced difficulties with complex and expensive tools to manage
              their sales and business operations. Our mission is to make sales
              management more accessible, efficient and profitable for all types
              of businesses.
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              During your trial period until{" "}
              <span className="font-medium">{trialEndDate}</span>, you have
              complete access to all premium features. Here are some
              recommendations on what to explore first:
            </Text>

            <br />

            <Text className="m-0 pl-4 text-left text-[#8D8D8D] text-sm leading-6">
              • Set up your product catalog and inventory
              <br />• Customize your sales area and processes
              <br />• Add team members and define permissions
              <br />• Register your suppliers and customers
              <br />• Explore advanced performance reports
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              If you prefer to ensure continuous access without interruptions,
              you can upgrade right now and enjoy all premium features without
              worrying about the trial period ending.
            </Text>

            <br />

            <Section>
              <Button
                className="inline-flex h-10 items-center justify-center rounded-[0.3rem] bg-[#FFFFE3] px-6 font-medium text-sm text-[#171717] no-underline transition-colors"
                href={upgradeUrl}
              >
                Upgrade now
              </Button>
            </Section>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              If you need support setting up your account or have any questions,
              I'm available to help. You can reply to this email or schedule a
              guidance session through{" "}
              <Link
                className="text-[#FF4F00] underline"
                href="https://calendly.com/strait/15min"
                style={{ color: "#FF4F00", textDecoration: "underline" }}
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

WelcomeTrial.PreviewProps = {
  name: "Leonardo",
  trialEndDate: "March 23, 2025",
  upgradeUrl: "https://app.usestrait.com/upgrade",
};

export default WelcomeTrial;
