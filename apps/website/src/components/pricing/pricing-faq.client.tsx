
import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@strait/ui/components/accordion";
import { Button } from "@strait/ui/components/button";
import { useId } from "react";

type FaqItem = {
  _id: string;
  question: string;
  answer: string | null;
};

type PricingFaqClientProps = {
  badge: string;
  title: string;
  description?: string;
  items: FaqItem[];
};

const PricingFaqClient = ({
  badge,
  title,
  description,
  items,
}: PricingFaqClientProps) => {
  const sectionId = useId();

  return (
    <section className="bg-background py-16 sm:py-20" id={sectionId}>
      <div className="mx-auto max-w-7xl px-6 lg:px-8">
        {/* Section Header - kicker pattern */}
        <div className="mx-auto flex max-w-3xl flex-col items-center gap-4 text-center">
          <span className="kicker">{badge}</span>
          <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
            {title}
          </h2>
          {description ? (
            <p className="max-w-2xl text-pretty text-muted-foreground text-sm leading-relaxed sm:text-base">
              {description}
            </p>
          ) : null}
        </div>

        {/* FAQ Accordion */}
        <div className="mx-auto mt-12 max-w-3xl sm:mt-16">
          <div className="border-border/50 border-y">
            <div className="border-border/50 border-x">
              <Accordion
                className="rounded-none border-0"
                defaultValue={[items[0]?._id]}
              >
                {items.map((item, index) => {
                  const isLast = index === items.length - 1;

                  return (
                    <AccordionItem
                      className={isLast ? "" : "border-border/50 border-b"}
                      key={item._id}
                      value={item._id}
                    >
                      <AccordionTrigger className="px-6 py-5 font-medium text-base text-foreground hover:bg-muted/30 hover:no-underline">
                        {item.question}
                      </AccordionTrigger>
                      <AccordionContent className="px-6 pb-6 text-base text-muted-foreground leading-relaxed">
                        {item.answer}
                      </AccordionContent>
                    </AccordionItem>
                  );
                })}
              </Accordion>
            </div>
          </div>
        </div>

        {/* CTA */}
        <div className="mt-10 flex justify-center sm:mt-12">
          <p className="text-center text-muted-foreground">
            Still have questions?{" "}
            <Button
              className="inline-flex"
              // biome-ignore lint/a11y/useAnchorContent: content provided by Button children
              render={<a href="mailto:leonardomso11@gmail.com" />}
              size="default"
              variant="link"
            >
              Contact our team
              <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
            </Button>
          </p>
        </div>
      </div>
    </section>
  );
};

export default PricingFaqClient;
