import PricingFaqClient from "./pricing-faq.client";

type FaqItem = {
  _id: string;
  question: string;
  answer: string | null;
};

export const PRICING_FAQ_ITEMS: FaqItem[] = [
  {
    _id: "faq-1",
    question: "Can I cancel anytime?",
    answer:
      "Yes. You can cancel your subscription at any time from your billing settings.",
  },
  {
    _id: "faq-2",
    question: "Do you offer yearly billing?",
    answer:
      "Yes. Yearly billing gives you a discounted monthly effective rate compared to monthly billing.",
  },
  {
    _id: "faq-3",
    question: "What happens if I hit plan limits?",
    answer:
      "You can upgrade at any time to unlock higher run volume and additional orchestration controls.",
  },
  {
    _id: "faq-4",
    question: "Do both plans include core runtime capabilities?",
    answer:
      "Yes. Both plans include core job execution, workflow orchestration, and operational visibility features.",
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
