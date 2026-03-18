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

const MAX_LINK_LENGTH = 40;

type OrganizationInviteProps = {
  name: string;
  orgName: string;
  inviteLink: string;
};

const OrganizationInvite = ({
  name,
  orgName = "Sales Team",
  inviteLink = "https://app.strait.com/invite/equipe/abc123",
}: OrganizationInviteProps) => (
  <Html>
    <Preview>Invitation to {orgName}</Preview>
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
            You've been invited to {orgName}
          </Heading>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
            {name} invited you to join the organization {orgName}. By accepting
            this invitation, you'll have access to the complete orchestration
            platform, where you can manage workflows, monitor jobs, and
            collaborate with your team.
          </Text>

          <br />

          <Section>
            <Button
              className="inline-flex h-10 items-center justify-center rounded-[0.3rem] bg-[#171717] px-6 font-medium text-sm text-white no-underline transition-colors"
              href={inviteLink}
            >
              Accept invitation
            </Button>
          </Section>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-[12px] leading-6">
            Or click this link to accept:{" "}
            <Link
              className="text-[#171717] underline"
              href={inviteLink}
              style={{ color: "#171717" }}
            >
              {inviteLink.slice(0, MAX_LINK_LENGTH)}...
            </Link>
          </Text>

          <br />

          <Text className="m-0 text-left text-[#8D8D8D] text-[12px] leading-6">
            This invitation expires in 7 days. If you didn't request this
            invitation, please ignore this email.
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

OrganizationInvite.PreviewProps = {
  name: "Alex Silva",
  orgName: "Sales Team",
  inviteLink: "https://app.strait.com/invite/equipe/abc123",
};

export default OrganizationInvite;
