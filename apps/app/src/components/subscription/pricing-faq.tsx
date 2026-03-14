import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@strait/ui/components/accordion";

export type FaqItem = {
  id: string;
  question: string;
  answer: string;
};

type PricingFAQProps = {
  title: string;
  description: string;
  items: FaqItem[];
};

const PricingFAQ = ({ title, description, items }: PricingFAQProps) => (
  <div className="w-full">
    <div className="flex w-full flex-col items-center gap-4 text-center">
      <h2 className="text-center text-3xl text-secondary-foreground tracking-tighter sm:text-4xl lg:text-5xl">
        {title}
      </h2>
      <p className="mx-auto max-w-2xl text-base text-muted-foreground leading-relaxed sm:leading-8">
        {description}
      </p>
    </div>

    <div className="mt-16">
      <Accordion>
        {items.map((item) => (
          <AccordionItem key={item.id} value={item.id}>
            <AccordionTrigger>{item.question}</AccordionTrigger>
            <AccordionContent>{item.answer}</AccordionContent>
          </AccordionItem>
        ))}
      </Accordion>
    </div>
  </div>
);

export default PricingFAQ;
