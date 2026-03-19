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
      "Yes. The Free plan gives you 1 organization, 3 projects, and 100 runs per day with no credit card required. It is designed for side projects and experimentation.",
  },
  {
    _id: "faq-2",
    question: "What are compute credits?",
    answer:
      "Compute credits cover the infrastructure cost of running your jobs. Each plan includes a monthly credit allowance. Starter includes $5, Pro includes $20, and Enterprise gets a custom allocation.",
  },
  {
    _id: "faq-3",
    question: "What happens when I use all my compute credits?",
    answer:
      "On Starter and Pro plans, additional runs are billed at a per-1,000-run overage rate. On Starter the overage rate is $2.00 per 1,000 runs. On Pro it is $1.50 per 1,000 runs. Enterprise plans have custom overage terms.",
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
      "Runs/day is a rate limit that controls how many jobs you can execute in a 24-hour period. Compute credits are a spending allowance that covers the infrastructure cost of those runs. You need both available capacity and credit to run jobs.",
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
      "Enterprise includes unlimited everything, SSO/SAML, full RBAC, audit logs, all regions, custom retention, dedicated support, AI BYOK, and custom overage and spending terms. Contact sales to discuss your requirements.",
  },
  {
    _id: "faq-9",
    question: "What regions are available?",
    answer:
      "Free plans run in a single region (iad). Starter plans can deploy across 3 regions. Pro plans have access to 6 regions. Enterprise plans can deploy to all available regions.",
  },
  {
    _id: "faq-10",
    question: "Can I switch plans?",
    answer:
      "Yes. You can upgrade or downgrade your plan at any time. When upgrading, you get immediate access to the new plan features. When downgrading, changes take effect at the end of your current billing period.",
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
