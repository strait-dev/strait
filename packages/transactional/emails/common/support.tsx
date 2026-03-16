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

type SupportProps = {
  name: string;
  email: string;
  subject: string;
  priority: "low" | "medium" | "high";
  environment: "production" | "development" | "staging";
  message: string;
  steps_to_reproduce: string;
  expected_result: string;
  actual_result: string;
  date?: string;
  createdAt?: string;
  lastLogin?: string;
};

const SupportEmail = ({
  name,
  email,
  subject = "Technical Problem",
  priority = "low",
  environment = "production",
  message = "I need help...",
  steps_to_reproduce = "1. I accessed...",
  expected_result = "It should...",
  actual_result = "But...",
  date = "March 12, 2024",
  createdAt = "January 10, 2024",
  lastLogin = "March 12, 2024",
}: SupportProps) => {
  const priorityTranslations = {
    low: "Low",
    medium: "Medium",
    high: "High",
  };

  const environmentTranslations = {
    production: "Production",
    development: "Development",
    staging: "Staging",
  };

  const priorityColors = {
    low: "bg-[#4F46E5]/10 text-[#4F46E5]",
    medium: "bg-[#FF4F00]/10 text-[#FF4F00]",
    high: "bg-[#E11D48]/10 text-[#E11D48]",
  };

  return (
    <Html>
      <Preview>New support request: {subject}</Preview>
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
              New Support Request
            </Heading>

            <br />

            <Text className="m-0 text-left text-[#252525] text-sm leading-6">
              A user sent a new support request with the subject:{" "}
              <strong>{subject}</strong>
            </Text>

            <br />

            {/* Support Info Section */}
            <Section>
              <Heading className="m-0 p-0 text-left font-medium text-[#252525] text-sm">
                Request Information
              </Heading>

              <br />

              <div>
                <Text className="m-0 text-left text-[#252525] text-sm leading-6">
                  <span className="font-medium">Priority:</span>{" "}
                  <span
                    className={`inline-block rounded-[0.1rem] px-2 py-1 font-medium text-xs ${priorityColors[priority]}`}
                  >
                    {priorityTranslations[priority]}
                  </span>
                </Text>
              </div>

              <br />

              <div>
                <Text className="m-0 text-left text-[#252525] text-sm leading-6">
                  <span className="font-medium">Environment:</span>{" "}
                  {environmentTranslations[environment]}
                </Text>
              </div>
            </Section>

            <br />

            {/* Problem Description */}
            <Section>
              <Heading className="m-0 p-0 text-left font-medium text-[#252525] text-sm">
                Problem Description
              </Heading>

              <br />

              <div className="rounded-[0.1rem] bg-[#F5F5F5] p-4">
                <Text className="m-0 text-left text-[#252525] text-sm leading-6">
                  {message}
                </Text>
              </div>
            </Section>

            <br />

            {/* Steps to Reproduce */}
            <Section>
              <Heading className="m-0 p-0 text-left font-medium text-[#252525] text-sm">
                Steps to Reproduce
              </Heading>

              <br />

              <div className="rounded-[0.1rem] bg-[#F5F5F5] p-4">
                <Text className="m-0 text-left text-[#252525] text-sm leading-6">
                  {steps_to_reproduce}
                </Text>
              </div>
            </Section>

            <br />

            {/* Expected vs Actual */}
            <Section>
              <div>
                <div>
                  <Heading className="m-0 p-0 text-left font-medium text-[#252525] text-sm">
                    Expected Result
                  </Heading>

                  <br />

                  <div className="rounded-[0.1rem] bg-[#F5F5F5] p-4">
                    <Text className="m-0 text-left text-[#252525] text-sm leading-6">
                      {expected_result}
                    </Text>
                  </div>
                </div>

                <br />

                <div>
                  <Heading className="m-0 p-0 text-left font-medium text-[#252525] text-sm">
                    Actual Result
                  </Heading>

                  <br />

                  <div className="rounded-[0.1rem] bg-[#F5F5F5] p-4">
                    <Text className="m-0 text-left text-[#252525] text-sm leading-6">
                      {actual_result}
                    </Text>
                  </div>
                </div>
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
                <Text className="m-0 text-left text-[#252525] text-sm leading-6">
                  <span className="font-medium">Name:</span> {name}
                </Text>
                <Text className="m-0 text-left text-[#252525] text-sm leading-6">
                  <span className="font-medium">Email:</span> {email}
                </Text>
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
                <Text className="m-0 text-left text-[#252525] text-sm leading-6">
                  <span className="font-medium">Request date:</span> {date}
                </Text>
                <Text className="m-0 text-left text-[#252525] text-sm leading-6">
                  <span className="font-medium">Account created:</span>{" "}
                  {createdAt}
                </Text>
                <Text className="m-0 text-left text-[#252525] text-sm leading-6">
                  <span className="font-medium">Last login:</span> {lastLogin}
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
            </Section>
          </Container>
        </Body>
      </Tailwind>
    </Html>
  );
};

SupportEmail.PreviewProps = {
  name: "John Silva",
  email: "user@example.com",
  subject: "Issue generating sales report",
  priority: "medium",
  environment: "production",
  message:
    "I'm trying to generate a sales report by region, but the page keeps loading indefinitely and never displays the results.",
  steps_to_reproduce:
    "1. Accessed the dashboard\n2. Clicked on 'Reports' > 'Sales by Region'\n3. Selected the period from 01/01/2025 to 15/03/2025\n4. Clicked 'Generate Report'",
  expected_result:
    "The report should be displayed with sales data by region for the selected period.",
  actual_result:
    "The page stays with the loading icon spinning indefinitely. Even after 10 minutes, no results are displayed.",
  date: "March 19, 2025",
  createdAt: "January 10, 2025",
  lastLogin: "March 19, 2025",
};

export default SupportEmail;
