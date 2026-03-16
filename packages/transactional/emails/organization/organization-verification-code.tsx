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

type Props = {
  name: string;
  organizationName: string;
  verificationCode: string;
  isPurgeOnly?: boolean;
};

const OrganizationVerificationCode = ({
  name,
  organizationName,
  verificationCode,
  isPurgeOnly = false,
}: Props) => (
  <Html>
    <Preview>
      {isPurgeOnly
        ? `Verification code for ${organizationName} organization data removal`
        : `Verification code for ${organizationName} organization deletion`}
    </Preview>
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
              src="https://app.usestrait.com/strait-logo-black.svg"
              width="150"
            />
          </Section>

          <br />

          <Heading className="m-0 p-0 text-left font-semibold text-[#252525] text-lg tracking-tight">
            {isPurgeOnly
              ? "Confirm organization data removal"
              : "Confirm organization deletion"}
          </Heading>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            Hello {name},
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            We received a request to{" "}
            {isPurgeOnly ? "remove all data from" : "delete"} the{" "}
            <span className="font-medium text-[#252525]">{organizationName}</span>{" "}
            organization from your Strait account. This action is{" "}
            <span className="font-medium text-[#252525]">permanent</span> and
            {isPurgeOnly
              ? " all personal data related to this organization, including workflows, jobs, schedules, and events, will be removed from our servers in compliance with data protection laws."
              : " all data related to this organization, including workflows, jobs, schedules, and events, will be removed from our system."}
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            To confirm {isPurgeOnly ? "the data removal" : "the deletion"} of
            your organization, use the verification code below:
          </Text>

          <br />

          <Section>
            <Container className="mx-auto rounded-[0.1rem] border border-[#EBEBEB] border-solid px-6 py-4 text-center">
              <Text className="m-0 font-bold font-mono text-2xl text-[#252525] tracking-widest">
                {verificationCode}
              </Text>
            </Container>
          </Section>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-[12px] leading-6">
            This code expires in 15 minutes. If you did not request{" "}
            {isPurgeOnly ? "the data removal" : "the deletion"}
            of your organization, please ignore this email or contact our{" "}
            <Link
              className="text-[#171717] underline"
              href="mailto:support@usestrait.com"
              style={{
                color: "#171717",
                textDecoration: "underline",
              }}
            >
              support
            </Link>{" "}
            immediately.
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

OrganizationVerificationCode.PreviewProps = {
  name: "Alex Silva",
  organizationName: "My Organization",
  verificationCode: "123456",
};

export default OrganizationVerificationCode;
