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

type FeedbackProps = {
  name: string;
  email: string;
  subject: string;
  message: string;
  date?: string;
  createdAt?: string;
  lastLogin?: string;
  plan?: "free" | "pro" | "enterprise";
};

const FeedbackEmail = ({
  name,
  email,
  subject = "I love Strait!",
  message = "I love Strait!",
  date = "March 12, 2024",
  createdAt = "January 10, 2024",
  lastLogin = "March 12, 2024",
  plan = "free",
}: FeedbackProps) => {
  const planTranslations = {
    free: "Free",
    pro: "Professional",
    enterprise: "Enterprise",
  };

  const planColors = {
    free: "bg-[#4F46E5]/10 text-[#4F46E5]",
    pro: "bg-[#FF4F00]/10 text-[#FF4F00]",
    enterprise: "bg-[#87CEEB]/10 text-[#0284C7]",
  };

  return (
    <Html>
      <Preview>New feedback received: {subject}</Preview>
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
              New Feedback Received
            </Heading>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              A user sent new feedback with the subject:{" "}
              <strong className="text-[#252525]">{subject}</strong>
            </Text>

            <br />

            {/* Feedback Content */}
            <Section>
              <Heading className="m-0 p-0 text-left font-medium text-[#252525] text-sm">
                User Message
              </Heading>

              <br />

              <div className="rounded-[0.1rem] bg-[#F5F5F5] p-4">
                <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
                  <span className="font-serif text-[#8D8D8D] text-xl">"</span>
                  {message}
                  <span className="font-serif text-[#8D8D8D] text-xl">"</span>
                </Text>
              </div>
            </Section>

            <br />

            {/* User Info Section */}
            <Section className="border-[#EBEBEB] border-t pt-4">
              <Heading className="m-0 p-0 text-left font-medium text-[#252525] text-sm">
                User Information
              </Heading>

              <br />

              <div className="space-y-2">
                <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
                  <span className="font-extrabold text-foreground">Name:</span>{" "}
                  {name}
                </Text>
                <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
                  <span className="font-extrabold text-foreground">Email:</span>{" "}
                  {email}
                </Text>
                {plan ? (
                  <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
                    <span className="font-extrabold text-foreground">
                      Plan:
                    </span>{" "}
                    <span
                      className={`inline-block rounded-[0.1rem] px-2 py-1 font-medium text-xs ${planColors[plan]}`}
                    >
                      {planTranslations[plan]}
                    </span>
                  </Text>
                ) : null}
              </div>
            </Section>

            <br />

            {/* Activity Info Section */}
            <Section className="border-[#EBEBEB] border-t pt-4">
              <Heading className="m-0 p-0 text-left font-medium text-[#252525] text-sm">
                Account Activity
              </Heading>

              <br />

              <div className="space-y-2">
                <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
                  <span className="font-extrabold text-foreground">
                    Feedback date:
                  </span>{" "}
                  {date}
                </Text>
                <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
                  <span className="font-extrabold text-foreground">
                    Account created:
                  </span>{" "}
                  {createdAt}
                </Text>
                <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
                  <span className="font-extrabold text-foreground">
                    Last login:
                  </span>{" "}
                  {lastLogin}
                </Text>
              </div>
            </Section>

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
};

FeedbackEmail.PreviewProps = {
  name: "João Silva",
  email: "user@example.com",
  subject: "Suggestion to improve sales reports",
  message:
    "I would like to suggest adding a comparative chart between the current month and the previous month in sales reports. This would help us better visualize growth and identify trends. Overall, I'm very satisfied with the platform, it has been essential for our sales growth!",
  date: "March 19, 2025",
  createdAt: "January 10, 2025",
  lastLogin: "March 19, 2025",
  plan: "pro",
};

export default FeedbackEmail;
