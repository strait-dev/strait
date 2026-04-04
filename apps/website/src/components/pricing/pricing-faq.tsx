import PricingFaqClient from "./pricing-faq.client.tsx";

type FaqItem = {
  _id: string;
  question: string;
  answer: string | null;
};

export const PRICING_FAQ_ITEMS: FaqItem[] = [
  {
    _id: "faq-1",
    question: "Is there really a free tier?",
    answer:
      "Yes. The Free plan gives you 1 organization, 2 projects, 3 members, and 5,000 runs per day with no credit card required. You also get 100 managed runs per month on the micro preset (10s timeout). It is designed for side projects and experimentation.",
  },
  {
    _id: "faq-2",
    question: "What are compute credits?",
    answer:
      "Compute credits cover the infrastructure cost of managed execution. On paid plans, your monthly credit equals your subscription price: Starter gets $19.99/mo and Pro gets $49.99/mo. The Free plan includes 100 managed runs per month on the micro preset (10s timeout) instead of a dollar credit. Enterprise plans get a custom allocation.",
  },
  {
    _id: "faq-3",
    question: "What happens when I use all my compute credits?",
    answer:
      "On Starter and Pro plans, additional runs are billed at $0.20 per 1,000 runs. You can configure spending limits to control overage. Enterprise plans have custom overage terms.",
  },
  {
    _id: "faq-4",
    question: "How does the 14-day free trial work?",
    answer:
      "Starter and Pro plans include a 14-day free trial. You get full access to all plan features during the trial. Your card is not charged until the trial ends, and you can cancel anytime before that.",
  },
  {
    _id: "faq-5",
    question: "Can I cancel anytime?",
    answer:
      "Yes. You can cancel your subscription at any time from your billing settings. Your plan stays active until the end of the current billing period.",
  },
  {
    _id: "faq-6",
    question: "What is the difference between runs/day and compute credits?",
    answer:
      "Runs/day is a rate limit that controls how many jobs you can execute in a 24-hour period (5,000 on Free, 25,000 on Starter, 100,000 on Pro). Compute credits are a separate spending allowance that covers the infrastructure cost of managed execution. You need both available capacity and credit to run managed jobs.",
  },
  {
    _id: "faq-7",
    question: "Do you offer annual billing?",
    answer:
      "Yes. Annual billing saves approximately 17% compared to monthly billing. You can switch between monthly and annual billing at any time.",
  },
  {
    _id: "faq-8",
    question: "What does Enterprise include?",
    answer:
      "Enterprise includes unlimited organizations, projects, members, runs, and concurrent runs. You also get 90-day retention, all regions, SSO/SAML, full RBAC, audit logs, dedicated support, AI BYOK, and custom compute and spending terms. Contact sales to discuss your requirements.",
  },
  {
    _id: "faq-9",
    question: "What regions are available?",
    answer:
      "Free plans run in a single region (iad). Starter plans can deploy across 6 regions. Pro and Enterprise plans have access to all available regions.",
  },
  {
    _id: "faq-10",
    question: "Can I switch plans?",
    answer:
      "Yes. You can upgrade or downgrade your plan at any time. When upgrading, you get immediate access to the new plan features. When downgrading, changes take effect at the end of your current billing period.",
  },
  {
    _id: "faq-11",
    question: "How is HTTP mode priced?",
    answer:
      "HTTP mode is available on Pro, Scale, and Enterprise plans. It is priced at $20 per 1 million runs ($0.00002 per run). With the Pro plan's included compute credit, you get approximately 2.5 million HTTP-mode runs before overage charges apply. Enterprise plans have custom HTTP mode pricing.",
  },
];

const PricingFaq = () => {
  return (
    <PricingFaqClient
      badge="FAQ"
      description="Everything you need to know before choosing a plan."
      items={PRICING_FAQ_ITEMS}
      title="Frequently asked questions"
    />
  );
};

export default PricingFaq;
