import {
  Body,
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
};

const TrialCheckin = ({ name = "Leonardo" }: Props) => {
  const previewText = "How is your Strait experience going?";

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
              This is Leo from Strait. I wanted to know how your experience with
              our sales management platform has been so far. Have you managed to
              set up your product catalog? Are you facing any difficulties?
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              I noticed you haven't explored our advanced reporting tool yet,
              which is excellent for businesses looking to increase their sales.
              With it, you can:
            </Text>

            <br />

            <Text className="m-0 pl-4 text-left text-[#8D8D8D] text-sm leading-6">
              • View real-time sales metrics <br />• Identify the most
              profitable products <br />• Analyze your customers' behavior{" "}
              <br />• Optimize your inventory based on demand forecasts
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              To give you an idea of the impact, one of our clients, Creative
              Store, increased their sales by 32% after implementing the
              strategies suggested by these reports.
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              I'm available to help you make the most of Strait and boost your
              results. If you have any questions or want to schedule a
              personalized guidance session, just reply to this email or visit
              our{" "}
              <Link
                className="text-[#FF4F00] underline"
                href="https://app.usestrait.com/help"
                style={{ color: "#FF4F00", textDecoration: "underline" }}
              >
                help center
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
          </Container>
        </Body>
      </Tailwind>
    </Html>
  );
};

TrialCheckin.PreviewProps = {
  name: "Leonardo",
};

export default TrialCheckin;
