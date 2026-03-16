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
};

const OrganizationDeleted = ({ name }: Props) => (
  <Html>
    <Preview>Your organization has been successfully deleted</Preview>
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
              src="/static/strait-logo-black.svg"
              width="150"
            />
          </Section>

          <br />

          <Heading className="m-0 p-0 text-left font-semibold text-[#252525] text-lg tracking-tight">
            Your organization has been successfully deleted
          </Heading>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            Hello {name},
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            We confirm that your organization has been permanently deleted from
            your Strait account. All data associated with this organization has
            been removed from our servers.
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            If you want to create a new organization, you can do so at any
            time by accessing the organizations menu at the top of Strait and
            clicking "Create new organization".
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            If you did not request this deletion or believe this was an error,
            please contact our support immediately.
          </Text>

          <br />

          <Hr className="mx-0 w-full border-[#EBEBEB] border-t" />

          <br />

          <Section>
            <Text className="m-0 text-left text-[#8D8D8D] text-[12px] leading-6">
              © 2026 Strait, All rights reserved
            </Text>
          </Section>
        </Container>
      </Body>
    </Tailwind>
  </Html>
);

OrganizationDeleted.PreviewProps = {
  name: "Alex Silva",
};

export default OrganizationDeleted;
