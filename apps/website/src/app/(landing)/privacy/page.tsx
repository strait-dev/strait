import type { Metadata } from "next";

import Shell from "@/components/layout/shell.tsx";
import { siteConfig } from "@/config/site.ts";
import { generateMetadata as generatePageMetadata } from "@/lib/metadata.ts";

export const generateMetadata = async (): Promise<Metadata> =>
  generatePageMetadata({
    title: "Privacy Policy",
    description: `${siteConfig.name} Privacy Policy`,
    path: "/privacy",
  });

const PrivacyPage = () => (
  <main className="pt-32">
    <Shell variant="default">
      <div className="mx-auto max-w-2xl text-center">
        <h1 className="mb-6 font-semibold text-4xl tracking-tight sm:text-5xl lg:text-6xl">
          Privacy Policy
        </h1>
        <p className="text-lg text-muted-foreground">
          This policy explains how Strait collects, uses, and protects your
          information.
        </p>
      </div>
    </Shell>

    <div className="container mx-auto max-w-2xl pb-24">
      <article className="prose prose-lg dark:prose-invert mx-auto">
        <h2>1. Information we collect</h2>
        <p>
          We collect information you provide directly, such as account details,
          profile information, and content you create while using Strait.
        </p>

        <h2>2. How we use information</h2>
        <p>
          We use your information to provide and improve the service, support
          your account, process billing, and communicate relevant updates.
        </p>

        <h2>3. Data retention</h2>
        <p>
          We retain data for as long as needed to operate the service and meet
          legal obligations. You may request deletion of your account data.
        </p>

        <h2>4. Security</h2>
        <p>
          We apply industry-standard safeguards to protect your information.
          However, no system can be guaranteed 100% secure.
        </p>

        <h2>5. Third-party services</h2>
        <p>
          Strait uses third-party providers for infrastructure, analytics, and
          payments. These providers process data under their own privacy terms.
        </p>

        <h2>6. Contact</h2>
        <p>
          For privacy questions, contact us at{" "}
          <a href="mailto:leonardomso11@gmail.com">leonardomso11@gmail.com</a>.
        </p>
      </article>
    </div>
  </main>
);

export default PrivacyPage;
