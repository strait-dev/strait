import {
  Body,
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

type GoodbyeEmailProps = {
  name: string;
};

const GoodbyeEmail = ({ name }: GoodbyeEmailProps) => (
  <Html>
    <Preview>Your Strait account has been deleted</Preview>
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
              alt="Strait Logo"
              className=""
              src="/static/strait-logo-black.svg"
              width="150"
            />
          </Section>

          <br />

          <Heading className="m-0 p-0 text-left font-semibold text-[#252525] text-lg tracking-tight">
            Your Strait account has been deleted
          </Heading>

          <br />

          <Section>
            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              Gosh, {name}, we'll miss you around here. We're sad to see you go
              and would like to know if there was something we could have done
              better.
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              If you'd like to share the reason for your departure or have any
              feedback for us, we'd be very grateful. You can contact us by
              email{" "}
              <Link
                href="mailto:support@usestrait.com"
                style={{ color: "#171717", textDecoration: "underline" }}
              >
                support@usestrait.com
              </Link>{" "}
              or through our support chat.
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              If you change your mind, you can always create a new account and
              return to our platform. Your information has been removed as
              requested, but we'll be here if you need us again.
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              Thank you for being part of Strait and for trusting us to
              simplify your workflow orchestration. We wish you success in your
              next steps!
            </Text>
          </Section>

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

GoodbyeEmail.PreviewProps = {
  name: "Leonardo",
};

export default GoodbyeEmail;
