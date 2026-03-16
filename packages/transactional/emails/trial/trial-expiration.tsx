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

const DAYS_REMAINING_THRESHOLD = 3;

type Props = {
  name: string;
  daysRemaining: number;
  upgradeUrl: string;
};

const TrialExpiration = ({
  name = "Leonardo",
  daysRemaining = 3,
  upgradeUrl = "https://app.usestrait.com/upgrade",
}: Props) => {
  let previewText = "";
  let mainText = "";

  if (daysRemaining === 1) {
    previewText = "Last day of your Strait trial period";
    mainText = `Hello ${name}, your Strait trial period ends tomorrow. To continue enjoying all the features of our sales management platform, upgrade to one of our premium plans.`;
  } else if (daysRemaining <= DAYS_REMAINING_THRESHOLD) {
    previewText = `Your Strait trial period ends in ${daysRemaining} days`;
    mainText = `Hello ${name}, your Strait trial period ends in ${daysRemaining} days. To continue enjoying all the features of our sales management platform, upgrade to one of our premium plans.`;
  } else {
    previewText = `Your Strait trial period ends in ${daysRemaining} days`;
    mainText = `Hello ${name}, your Strait trial period ends in ${daysRemaining} days. To continue enjoying all the features of our sales management platform, upgrade to one of our premium plans.`;
  }

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
              {mainText}
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              With Strait, you can:
            </Text>

            <br />

            <Text className="m-0 pl-4 text-left text-[#8D8D8D] text-sm leading-6">
              • Manage your product catalog and inventory with ease
              <br />• Automate sales processes and track negotiations
              <br />• Access detailed performance and trend reports
              <br />• Integrate with marketplaces, ERPs and payment methods
              <br />• Increase your sales with advanced analytics tools
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              Don't lose access to the tools and data you've already configured.
              Upgrade now and continue boosting your business with Strait.
            </Text>

            <br />

            <Section>
              <Button
                className="inline-flex h-10 items-center justify-center rounded-[0.3rem] bg-[#FF4F00] px-6 font-medium text-sm text-white no-underline transition-colors"
                href={upgradeUrl}
              >
                Upgrade now
              </Button>
            </Section>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              If you prefer, we can schedule a quick conversation to discuss
              your specific needs and ensure you choose the most suitable plan.
              Just reply to this email or schedule through{" "}
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
              <Text className="m-0 text-left text-[#8D8D8D] text-[12px] leading-6">
                CNPJ 59.888.832/0001-39
              </Text>
              <Text className="m-0 text-left text-[#8D8D8D] text-[12px] leading-6">
                Av. Princesa Isabel — Vitória, ES, Brazil 29.010-361
              </Text>
            </Section>
          </Container>
        </Body>
      </Tailwind>
    </Html>
  );
};

TrialExpiration.PreviewProps = {
  name: "Leonardo",
  daysRemaining: 3,
  upgradeUrl: "https://app.usestrait.com/upgrade",
};

export default TrialExpiration;
