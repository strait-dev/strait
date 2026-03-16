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

type Props = {
  name: string;
  url: string;
};

const DeleteAccount = ({ name, url }: Props) => (
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
    <Preview>Confirm the deletion of your account on Strait</Preview>
    <Tailwind>
      <Body className="mx-auto my-auto bg-[#FFFFFF] font-sans">
        <Container className="mx-auto my-10 max-w-[500px] rounded-[0.1rem] border border-[#EBEBEB] border-solid px-10 py-8">
          <Section>
            <Img
              alt="Strait Logo"
              className="mb-6"
              src="https://mwesulbn1k.ufs.sh/f/DedoMBfQiCy9vOEDu2YCvLugTtO8VEnoywN2DbkUr6QB1MP3"
              width="150"
            />
          </Section>

          <Heading className="m-0 mb-4 p-0 text-left font-semibold text-[#252525] text-lg tracking-tight">
            Confirm the deletion of your account
          </Heading>

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            Hello {name}
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            We received a request to delete your account on Strait. This action
            is <span className="font-extrabold text-foreground">permanent</span>{" "}
            and all your data will be removed from our system.
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            To confirm the deletion of your account, click the button below:
          </Text>

          <br />

          <Section>
            <Button
              className="inline-flex h-10 items-center justify-center rounded-[0.3rem] bg-[#FF4F00] px-6 font-medium text-sm text-white no-underline transition-colors"
              href={url}
            >
              Confirm deletion
            </Button>
          </Section>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-[12px] leading-6">
            This link expires in 24 hours. If you did not request the deletion
            of your account, please ignore this email or contact our{" "}
            <Link
              className="text-[#FF4F00] underline"
              href="mailto:support@usestrait.com"
              style={{ color: "#FF4F00" }}
            >
              support
            </Link>{" "}
            immediately.
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-[12px] leading-6">
            If the button does not work, copy and paste this link in your
            browser:{" "}
            <Link
              className="text-[#FF4F00] underline"
              href={url}
              style={{ color: "#FF4F00" }}
            >
              {url}
            </Link>
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

DeleteAccount.PreviewProps = {
  name: "Leonardo Santos",
  url: "https://app.usestrait.com/delete-account?token=abc123",
};

export default DeleteAccount;
