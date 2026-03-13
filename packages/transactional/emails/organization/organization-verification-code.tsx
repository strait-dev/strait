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
        ? `Verification code for ${organizationName} store data removal`
        : `Verification code for ${organizationName} store deletion`}
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
      <Body className="mx-auto my-auto bg-white font-sans">
        <Container className="mx-auto my-10 max-w-[500px] rounded-[0.1rem] border border-gray-200 border-solid px-10 py-8">
          <Section>
            <Img
              alt="Strait"
              className=""
              src="https://mwesulbn1k.ufs.sh/f/DedoMBfQiCy9vOEDu2YCvLugTtO8VEnoywN2DbkUr6QB1MP3"
              width="150"
            />
          </Section>

          <br />

          <Heading className="m-0 p-0 text-left font-semibold text-black text-lg tracking-tight">
            {isPurgeOnly
              ? "Confirm store data removal"
              : "Confirm store deletion"}
          </Heading>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            Hello {name},
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            We received a request to{" "}
            {isPurgeOnly ? "remove all data from" : "delete"} the{" "}
            <span className="font-medium text-black">{organizationName}</span>{" "}
            store from your Strait account. This action is{" "}
            <span className="font-medium text-black">permanent</span> and
            {isPurgeOnly
              ? " all personal data related to this store will be removed from our servers in compliance with data protection laws."
              : " all data related to this store will be removed from our system."}
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            To confirm {isPurgeOnly ? "the data removal" : "the deletion"} of
            your store, use the verification code below:
          </Text>

          <br />

          <Section>
            <Container className="mx-auto rounded-[0.1rem] border border-gray-200 border-solid px-6 py-4 text-center">
              <Text className="m-0 font-bold font-mono text-2xl text-black tracking-widest">
                {verificationCode}
              </Text>
            </Container>
          </Section>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-[12px] leading-6">
            This code expires in 15 minutes. If you did not request{" "}
            {isPurgeOnly ? "the data removal" : "the deletion"}
            of your store, please ignore this email or contact our{" "}
            <Link
              className="text-[#FF4F00] underline"
              href="mailto:support@usestrait.com"
              style={{
                color: "#FF4F00",
                textDecoration: "underline",
              }}
            >
              support
            </Link>{" "}
            immediately.
          </Text>

          <br />

          <Hr className="mx-0 w-full border-gray-200 border-t" />

          <br />
        </Container>
      </Body>
    </Tailwind>
  </Html>
);

OrganizationVerificationCode.PreviewProps = {
  name: "Alex Silva",
  organizationName: "My Store",
  verificationCode: "123456",
};

export default OrganizationVerificationCode;
