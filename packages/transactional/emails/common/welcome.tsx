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

type WelcomeEmailProps = {
  name: string;
};

const WelcomeEmail = ({ name }: WelcomeEmailProps) => (
  <Html>
    <Preview>Welcome to Strait!</Preview>
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
              src="https://mwesulbn1k.ufs.sh/f/DedoMBfQiCy9vOEDu2YCvLugTtO8VEnoywN2DbkUr6QB1MP3"
              width="150"
            />
          </Section>

          <br />

          <Heading className="m-0 p-0 text-left font-semibold text-[#252525] text-lg tracking-tight">
            Welcome to Strait!
          </Heading>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            {name ? `Hello ${name},` : "Hello,"} welcome to Strait!
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            We're very happy to have you with us. With Strait, you'll have
            access to powerful tools to manage your workflows, automate jobs,
            and monitor your operations in real time.
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            Here are some things you can do now:
          </Text>

          <br />

          <Section>
            <Text className="m-0 text-[#8D8D8D] text-sm leading-6">
              • Set up your first{" "}
              <Link
                href="https://app.usestrait.com/workflows"
                style={{ color: "#FF4F00", textDecoration: "underline" }}
              >
                workflow
              </Link>
              <br />• Configure your{" "}
              <Link
                href="https://app.usestrait.com/settings"
                style={{ color: "#FF4F00", textDecoration: "underline" }}
              >
                team settings
              </Link>
              <br />• Create{" "}
              <Link
                href="https://app.usestrait.com/schedules"
                style={{ color: "#FF4F00", textDecoration: "underline" }}
              >
                automated schedules
              </Link>
              <br />• Monitor your{" "}
              <Link
                href="https://app.usestrait.com/runs"
                style={{ color: "#FF4F00", textDecoration: "underline" }}
              >
                runs and events
              </Link>
            </Text>
          </Section>

          <br />

          <Section>
            <Button
              className="inline-flex h-10 items-center justify-center rounded-[0.3rem] bg-[#FFFFE3] px-6 font-medium text-sm text-[#171717] no-underline transition-colors"
              href="https://app.usestrait.com"
            >
              Access my account
            </Button>
          </Section>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-[12px] leading-6">
            If you have any questions, just reply to this email or contact our
            support team. We're here to help you succeed!
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

WelcomeEmail.PreviewProps = {
  name: "Leonardo Santos",
};

export default WelcomeEmail;
