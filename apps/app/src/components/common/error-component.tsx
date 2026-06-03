import { HugeiconsIcon } from "@hugeicons/react";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@strait/ui/components/accordion";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@strait/ui/components/alert";
import { Button } from "@strait/ui/components/button";
import { CodeBlock } from "@strait/ui/components/code-block";
import {
  DescriptionDetails,
  DescriptionList,
  DescriptionTerm,
} from "@strait/ui/components/description-list";
import { useQueryErrorResetBoundary } from "@tanstack/react-query";
import { Link, useRouter } from "@tanstack/react-router";
import { useEffect } from "react";
import { AlertIcon } from "@/lib/icons";
import { captureException } from "@/lib/sentry";

const ErrorComponent = ({ error }: { error: Error }) => {
  const router = useRouter();

  const queryClientErrorBoundary = useQueryErrorResetBoundary();

  const isDev = process.env.NODE_ENV !== "production";

  useEffect(() => {
    queryClientErrorBoundary.reset();
  }, [queryClientErrorBoundary]);

  useEffect(() => {
    captureException(error, {
      tags: {
        location: "error_boundary",
        error_type: error.name,
      },
    });
  }, [error]);

  return (
    <div className="mt-8 flex items-center justify-center p-4">
      <div className="w-full max-w-md">
        <Alert variant={"destructive"}>
          <HugeiconsIcon className="size-4" icon={AlertIcon} />
          <AlertTitle>Oops! Something went wrong</AlertTitle>
          <AlertDescription>
            We're sorry, but the website has encountered an unexpected issue
          </AlertDescription>
        </Alert>
        <div className="mt-4 space-y-4">
          <Button
            className="w-full"
            onClick={() => {
              router.invalidate();
            }}
          >
            Try again
          </Button>
          <Button
            className="w-full"
            render={<Link preload="intent" to="/" />}
            variant={"outline"}
          >
            Return to home
          </Button>
          {isDev ? (
            <Accordion className="w-full">
              <AccordionItem value="error-details">
                <AccordionTrigger>View error details</AccordionTrigger>
                <AccordionContent>
                  <DescriptionList size="sm">
                    <DescriptionTerm>Error details</DescriptionTerm>
                    <DescriptionDetails>{error.message}</DescriptionDetails>
                    <DescriptionTerm>Error trace</DescriptionTerm>
                    <DescriptionDetails>
                      <CodeBlock
                        code={error.stack ?? error.message}
                        copyable={false}
                        maxHeight={320}
                        wrap
                      />
                    </DescriptionDetails>
                  </DescriptionList>
                </AccordionContent>
              </AccordionItem>
            </Accordion>
          ) : null}
        </div>
      </div>
    </div>
  );
};

export default ErrorComponent;
