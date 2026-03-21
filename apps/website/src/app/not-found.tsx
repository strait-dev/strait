import type { Metadata } from "next";

import ErrorDisplay from "@/components/error-display.tsx";
import Footer from "./(landing)/components/common/footer/footer.tsx";
import Header from "./(landing)/components/common/header/header.tsx";

export const metadata: Metadata = {
  title: "Page Not Found",
  description: "The page you're looking for doesn't exist or has been moved.",
};

const NotFound = () => (
  <>
    <Header />
    <main>
      <ErrorDisplay
        actions={[
          { label: "Go home", href: "/", variant: "default" },
          { label: "Read the blog", href: "/blog", variant: "outline" },
        ]}
        code="404"
        description="The page you're looking for doesn't exist or has been moved. Check the URL, or head back to explore Strait."
        title="Page not found"
      />
    </main>
    <Footer />
  </>
);

export default NotFound;
