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

type PasswordUpdateProps = {
  name: string;
};

const PasswordUpdate = ({ name }: PasswordUpdateProps) => (
  <Html>
    <Preview>Your password has been changed successfully</Preview>
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
              className="mb-6"
              src="https://mwesulbn1k.ufs.sh/f/DedoMBfQiCy9vOEDu2YCvLugTtO8VEnoywN2DbkUr6QB1MP3"
              width="150"
            />
          </Section>

          <Heading className="m-0 mb-4 p-0 text-left font-semibold text-[#252525] text-lg tracking-tight">
            Password changed successfully
          </Heading>

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            Hello {name},
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            Your Strait account password was successfully changed on{" "}
            {new Date().toLocaleDateString("en-US")}. This message is just to
            confirm that the change has been made.
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            If you didn't make this change, please contact our support
            immediately through email at{" "}
            <Link
              className="text-[#FF4F00] underline"
              href="mailto:support@usestrait.com"
              style={{ color: "#FF4F00" }}
            >
              support@usestrait.com
            </Link>{" "}
            or visit our Help Center.
          </Text>

          <br />

          <Section>
            <Button
              className="inline-flex h-10 items-center justify-center rounded-[0.3rem] bg-[#FF4F00] px-6 font-medium text-sm text-white no-underline transition-colors"
              href="https://app.usestrait.com/help"
            >
              Help Center
            </Button>
          </Section>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-[12px] leading-6">
            For security, we recommend that you log out of all devices and log
            in again with your new password. It's also recommended to regularly
            check your account activity.
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

PasswordUpdate.PreviewProps = {
  name: "Leonardo Santos",
};

export default PasswordUpdate;
