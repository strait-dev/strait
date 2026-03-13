import {
  Body,
  Container,
  Font,
  Head,
  Heading,
  Hr,
  Html,
  Img,
  Preview,
  Section,
  Tailwind,
  Text,
} from "@react-email/components";

type Props = {
  name: string;
  organizationName: string;
};

const OrganizationPurged = ({ name, organizationName }: Props) => (
  <Html>
    <Preview>
      {organizationName} store data has been permanently removed
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
            Store data permanently removed
          </Heading>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            Hello {name},
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            We confirm that all data associated with the{" "}
            <span className="font-medium text-black">{organizationName}</span>{" "}
            store has been permanently removed from our servers as requested.
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            This action is irreversible and all data, including order history,
            products, customers, and settings have been completely erased in
            compliance with data protection laws.
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            If you did not request this action or believe this was an error,
            please contact our support immediately.
          </Text>

          <br />

          <Hr className="mx-0 w-full border-gray-200 border-t" />

          <br />
        </Container>
      </Body>
    </Tailwind>
  </Html>
);

OrganizationPurged.PreviewProps = {
  name: "Alex Silva",
  organizationName: "My Store",
};

export default OrganizationPurged;
