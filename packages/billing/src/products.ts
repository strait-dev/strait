export type Plan = {
  name: string;
  description: string;
  prices: {
    monthly: number;
    yearly: number;
  };
  features: string[];
};

export const PLANS: Record<"personal" | "pro", Plan> = {
  personal: {
    name: "Personal",
    description: "For individual writers and creators.",
    prices: {
      monthly: 1900,
      yearly: 19000,
    },
    features: [
      "AI interview",
      "Multi-draft generation",
      "Style guidance",
      "Export options",
    ],
  },
  pro: {
    name: "Pro",
    description: "For teams and power users who need advanced workflows.",
    prices: {
      monthly: 4900,
      yearly: 49000,
    },
    features: [
      "Everything in Personal",
      "Advanced editing tools",
      "Priority support",
      "Higher usage limits",
    ],
  },
};

export function formatPrice(cents: number, currency = "USD"): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency,
    maximumFractionDigits: 0,
  }).format(cents / 100);
}

export function formatPriceWithCents(cents: number, currency = "USD"): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency,
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(cents / 100);
}
