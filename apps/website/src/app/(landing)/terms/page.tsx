import type { Metadata } from "next";

import Shell from "@/components/layout/shell.tsx";
import { siteConfig } from "@/config/site.ts";
import { generateMetadata as generatePageMetadata } from "@/lib/metadata.ts";

export const generateMetadata = async (): Promise<Metadata> =>
  generatePageMetadata({
    title: "Terms of Service",
    description: `${siteConfig.name} Terms of Service`,
    path: "/terms",
  });

const TermsPage = () => (
  <main className="pt-32 sm:pt-40">
    <Shell variant="default">
      <div className="mx-auto max-w-2xl text-center">
        <h1 className="mb-6 text-4xl sm:text-5xl lg:text-6xl">
          Terms of Service
        </h1>
        <p className="text-lg text-muted-foreground">
          These terms govern your access to and use of Strait.
        </p>
      </div>
    </Shell>

    <div className="container mx-auto max-w-2xl pb-24">
      <article className="prose prose-lg dark:prose-invert mx-auto">
        <h2>1. Acceptance of terms</h2>
        <p>
          By using Strait, you agree to these terms and all applicable laws and
          regulations.
        </p>

        <h2>2. Account responsibilities</h2>
        <p>
          You are responsible for maintaining the security of your account and
          for all activities that occur under it.
        </p>

        <h2>3. Acceptable use</h2>
        <p>
          You agree not to misuse the service, attempt unauthorized access, or
          use Strait for unlawful or harmful activities.
        </p>

        <h2>4. Billing and subscriptions</h2>
        <p>
          Paid plans are billed according to the pricing terms at checkout.
          Subscriptions renew until cancelled.
        </p>

        <h2>5. Intellectual property</h2>
        <p>
          Strait and its related assets are protected by intellectual property
          laws. You retain rights to content you create.
        </p>

        <h2>6. Termination</h2>
        <p>
          We may suspend or terminate access if these terms are violated or if
          required for legal or security reasons.
        </p>

        <h2>7. Contact</h2>
        <p>
          For questions about these terms, contact{" "}
          <a href="mailto:leonardomso11@gmail.com">leonardomso11@gmail.com</a>.
        </p>
      </article>
    </div>
  </main>
);

export default TermsPage;
