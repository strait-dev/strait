import {
  Body,
  Button,
  Container,
  Font,
  Head,
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
  trialEndDate: string;
  recommendedPlan?: string;
  upgradeUrl: string;
};

const TrialEndingRecommendation = ({
  name = "Leonardo",
  trialEndDate = "March 23, 2025",
  recommendedPlan = "Professional",
  upgradeUrl = "https://app.usestrait.com/upgrade?plan=pro",
}: Props) => {
  const previewText = "My personal recommendation for your store";

  return (
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
      <Preview>{previewText}</Preview>
      <Tailwind>
        <Body className="mx-auto my-auto bg-[#FFFFFF] font-sans">
          <Container className="mx-auto my-10 max-w-[500px] rounded-[0.1rem] border border-[#EBEBEB] border-solid px-10 py-8">
            <Section>
              <Img
                alt="Strait Logo"
                className=""
                src="https://mwesulbn1k.ufs.sh/f/DedoMBfQiCy9vOEDu2YCvLugTtO8VEnoywN2DbkUr6QB1MP3"
                width="150"
              />
            </Section>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              Hello {name},
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              Your Strait trial ends on{" "}
              <span className="font-medium text-[#8D8D8D]">{trialEndDate}</span>{" "}
              and I'd like to share a personalized recommendation for your
              business:
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              I've analyzed how you've been using our sales management platform
              and, considering your business volume of products and
              transactions, I believe the {recommendedPlan} plan would be the
              ideal choice for your needs.
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              With this plan, you'll have access to:
            </Text>

            <br />

            <Text className="m-0 pl-4 text-left text-[#8D8D8D] text-sm leading-6">
              • All the tools you're currently using <br />• Marketing
              automation for lost sales recovery <br />• Advanced performance
              reports and customer analysis <br />• Premium integrations with
              marketplaces and payment platforms <br />• Priority 24/7 support
              with guaranteed response time
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              To put this in perspective, businesses similar to yours that
              migrated to this plan recorded an average increase of $3,200 in
              monthly sales, with an investment of only $197/month –
              representing a return on investment of over 16x!
            </Text>

            <br />

            <Section>
              <Button
                className="inline-flex h-10 items-center justify-center rounded-[0.3rem] bg-[#FF4F00] px-6 font-medium text-sm text-white no-underline transition-colors"
                href={upgradeUrl}
              >
                Upgrade to {recommendedPlan} plan
              </Button>
            </Section>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              If you prefer, we can schedule a quick conversation to discuss
              your specific needs and ensure you choose the most suitable plan.
              Just reply to this email with your availability or schedule
              directly through{" "}
              <Link
                className="text-[#FF4F00] underline"
                href="https://calendly.com/strait/15min"
                style={{ color: "#FF4F00", textDecoration: "underline" }}
              >
                our calendar
              </Link>
              .
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              Best regards,
            </Text>

            <br />

            <Text className="m-0 text-left text-[#8D8D8D] text-sm leading-6">
              Leo
              <br />
              Founder of Strait
            </Text>

            <br />

            <Hr className="mx-0 w-full border-[#EBEBEB] border-t" />

            <br />
          </Container>
        </Body>
      </Tailwind>
    </Html>
  );
};

TrialEndingRecommendation.PreviewProps = {
  name: "Leonardo",
  trialEndDate: "March 23, 2025",
  recommendedPlan: "Professional",
  upgradeUrl: "https://app.usestrait.com/upgrade?plan=pro",
};

export default TrialEndingRecommendation;
