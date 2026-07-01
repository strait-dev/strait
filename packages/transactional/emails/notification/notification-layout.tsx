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
import type { ReactNode } from "react";

type NotificationLayoutProps = {
  preview: string;
  heading: string;
  children: ReactNode;
};

export const NotificationLayout = ({
  preview,
  heading,
  children,
}: NotificationLayoutProps) => (
  <Html>
    <Preview>{preview}</Preview>
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
        <Container className="mx-auto my-10 max-w-[600px] rounded-[0.1rem] border border-[#EBEBEB] border-solid px-10 py-8">
          <Section>
            <Img alt="Strait" src="/static/strait-logo-black.svg" width="150" />
          </Section>

          <br />

          <Heading className="m-0 p-0 text-left font-semibold text-[#252525] text-lg tracking-tight">
            {heading}
          </Heading>

          <br />

          {children}

          <br />

          <Hr className="mx-0 w-full border-[#EBEBEB] border-t" />

          <br />

          <Section>
            <Text className="m-0 text-left text-[#8D8D8D] text-[12px] leading-6">
              This is an automated notification from Strait.
            </Text>
          </Section>
        </Container>
      </Body>
    </Tailwind>
  </Html>
);

export const NotificationText = ({ children }: { children: ReactNode }) => (
  <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
    {children}
  </Text>
);

export const MetricTable = ({ children }: { children: ReactNode }) => (
  <Section>
    <table
      style={{
        borderCollapse: "collapse",
        width: "100%",
      }}
    >
      <tbody>{children}</tbody>
    </table>
  </Section>
);

export const MetricRow = ({
  label,
  value,
}: {
  label: string;
  value: string;
}) => (
  <tr>
    <td
      style={{
        border: "1px solid #dddddd",
        color: "#8D8D8D",
        padding: "8px",
      }}
    >
      {label}
    </td>
    <td
      style={{
        border: "1px solid #dddddd",
        color: "#252525",
        padding: "8px",
      }}
    >
      {value}
    </td>
  </tr>
);
