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

const TrialValueReminder = ({
  name = "Leonardo",
  upgradeUrl = "https://app.usestrait.com/upgrade",
}: Props) => {
  const previewText = "What have you already achieved with Strait?";

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
              I'm very happy to know that you're enjoying our sales management
              platform!
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              <span className="font-medium">
                Can you help me understand how we can boost your sales even
                further?
              </span>{" "}
              What's the biggest challenge you're currently facing? If you have
              any specific feature you'd like to explore better or any questions
              about how to optimize your sales processes, I'm here to help.
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              It's worth noting that businesses similar to yours, who continued
              with Strait after the trial period, recorded an average increase
              of 42% in sales in the first 3 months. For example, Modern Store
              increased their revenue by 37% right after fully implementing our
              platform into their sales processes.
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
              If you need support to increase your sales or have any questions
              about how to make the most of our platform, you can schedule a
              personalized session with me through{" "}
              <Link
                className="text-[#FF4F00] underline"
                href="https://cal.com/leostrait/15min"
                style={{ color: "#FF4F00", textDecoration: "underline" }}
              >
                our calendar
              </Link>{" "}
              or simply reply to this email.
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
          </Container>
        </Body>
      </Tailwind>
    </Html>
  );
};

TrialValueReminder.PreviewProps = {
  name: "Leonardo",
  upgradeUrl: "https://app.usestrait.com/upgrade",
};

export default TrialValueReminder;
