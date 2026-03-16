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

type ContactProps = {
  name: string;
  email: string;
  phone: string;
  company?: string;
  subject: string;
  message: string;
  date: string;
  ipAddress?: string;
};

const ContactEmail = ({
  name = "Visitor",
  email = "email@example.com",
  phone = "(00) 00000-0000",
  company = "",
  subject = "Website contact",
  message = "I would like more information about Strait.",
  date = "March 12, 2024",
  ipAddress = "",
}: ContactProps) => {
  return (
    <Html>
      <Preview>New contact received: {subject}</Preview>
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
              New Website Contact
            </Heading>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              A visitor sent a message with the subject:{" "}
              <strong className="text-[#252525]">{subject}</strong>
            </Text>

            <br />

            {/* Contact Message Section */}
            <Section>
              <Heading className="m-0 p-0 text-left font-medium text-[#252525] text-sm">
                Message
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

            {/* Contact Info Section */}
            <Section className="border-[#EBEBEB] border-t pt-4">
              <Heading className="m-0 p-0 text-left font-medium text-[#252525] text-sm">
                Contact Information
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
                <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
                  <span className="font-extrabold text-foreground">Phone:</span>{" "}
                  {phone}
                </Text>
                {company ? (
                  <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
                    <span className="font-extrabold text-foreground">
                      Company:
                    </span>{" "}
                    {company}
                  </Text>
                ) : null}
              </div>
            </Section>

            <br />

            {/* Additional Info Section */}
            <Section className="border-[#EBEBEB] border-t pt-4">
              <Heading className="m-0 p-0 text-left font-medium text-[#252525] text-sm">
                Additional Information
              </Heading>

              <br />

              <div className="space-y-2">
                <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
                  <span className="font-extrabold text-foreground">
                    Contact date:
                  </span>{" "}
                  {date}
                </Text>
                {ipAddress ? (
                  <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
                    <span className="font-extrabold text-foreground">
                      IP Address:
                    </span>{" "}
                    {ipAddress}
                  </Text>
                ) : null}
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

ContactEmail.PreviewProps = {
  name: "Maria Silva",
  email: "maria.silva@example.com",
  phone: "(11) 98765-4321",
  company: "Example Company Ltd",
  subject: "Demo request",
  message:
    "Hello, I would like to schedule a system demo to evaluate if it meets my company's needs. I'm looking for a solution that can integrate sales, inventory, and finance on a single platform.",
  date: "April 15, 2024",
};

export default ContactEmail;
