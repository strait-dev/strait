"use client";

import { Button } from "@strait/ui/components/button";
import { cn } from "@strait/ui/utils";
import { useCallback, useState } from "react";

import type { TestimonialItem } from "./testimonial.tsx";

type TestimonialCarouselProps = {
  testimonials: TestimonialItem[];
};

const TestimonialCarousel = ({ testimonials }: TestimonialCarouselProps) => {
  const [activeIndex, setActiveIndex] = useState(0);

  const prev = useCallback(() => {
    setActiveIndex((i) => Math.max(0, i - 1));
  }, []);

  const next = useCallback(() => {
    setActiveIndex((i) => Math.min(testimonials.length - 1, i + 1));
  }, [testimonials.length]);

  const current = testimonials[activeIndex];
  if (!current) {
    return null;
  }

  const authorInfo = [current.authorPosition, current.authorCompany]
    .filter(Boolean)
    .join(" at ");

  return (
    <div className="relative">
      <div className="grid min-h-[400px] grid-cols-1 lg:min-h-[500px] lg:grid-cols-[400px_1fr]">
        <div className="relative hidden overflow-hidden bg-muted lg:block">
          {testimonials.map((t, index) => (
            <div
              className={cn(
                "absolute inset-0 transition-opacity duration-300",
                activeIndex === index ? "opacity-100" : "opacity-0"
              )}
              key={t._id}
            >
              {t.avatar?.url ? (
                <img
                  alt={`Photo of ${t.authorName ?? "testimonial author"}`}
                  className="absolute inset-0 h-full w-full object-cover"
                  decoding="async"
                  loading="lazy"
                  src={t.avatar.url}
                />
              ) : (
                <div className="flex h-full items-center justify-center">
                  <span className="text-4xl text-muted-foreground/30">
                    {t.authorName
                      ?.split(" ")
                      .map((n: string) => n[0])
                      .join("")}
                  </span>
                </div>
              )}
            </div>
          ))}
          <div className="absolute inset-x-0 bottom-0 h-24 bg-gradient-to-t from-background/80 to-transparent" />
        </div>

        <div className="flex flex-col justify-center p-8 sm:p-12 lg:p-16">
          <div className="animate-fade-in-up" key={current._id}>
            <svg
              className="mb-6 text-primary/20"
              fill="none"
              height="48"
              viewBox="0 0 24 24"
              width="48"
              xmlns="http://www.w3.org/2000/svg"
            >
              <path
                d="M9.135 5.015C5.497 6.45 3 9.515 3 13.135c0 2.58 1.74 4.365 3.78 4.365 1.92 0 3.42-1.5 3.42-3.36 0-1.8-1.32-3.18-3-3.42.36-2.22 2.28-4.26 4.8-5.205L9.135 5.015zm10.2 0C15.697 6.45 13.2 9.515 13.2 13.135c0 2.58 1.74 4.365 3.78 4.365 1.92 0 3.42-1.5 3.42-3.36 0-1.8-1.32-3.18-3-3.42.36-2.22 2.28-4.26 4.8-5.205L19.335 5.015z"
                fill="currentColor"
              />
            </svg>

            <blockquote>
              <p className="text-foreground text-xl leading-relaxed sm:text-2xl lg:text-3xl">
                &ldquo;{current.text}&rdquo;
              </p>
            </blockquote>

            <div className="mt-8 flex items-center gap-4">
              <div className="size-12 shrink-0 overflow-hidden rounded-full bg-muted lg:hidden">
                {current.avatar?.url ? (
                  <img
                    alt={`Photo of ${current.authorName ?? "testimonial author"}`}
                    className="object-cover"
                    decoding="async"
                    height={48}
                    loading="lazy"
                    src={current.avatar.url}
                    width={48}
                  />
                ) : (
                  <div className="flex h-full items-center justify-center">
                    <span className="font-semibold text-muted-foreground text-sm">
                      {current.authorName
                        ?.split(" ")
                        .map((n: string) => n[0])
                        .join("")}
                    </span>
                  </div>
                )}
              </div>
              <div>
                <p className="font-semibold text-foreground">
                  {current.authorName}
                </p>
                {authorInfo ? (
                  <p className="text-muted-foreground text-sm">{authorInfo}</p>
                ) : null}
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className="flex items-center justify-between border-border/50 border-t px-4 py-4 sm:px-6 lg:px-8">
        <div className="flex items-center gap-2">
          {testimonials.map((t, index) => (
            <button
              aria-label={`Go to testimonial ${String(index + 1)}`}
              className={cn(
                "h-2 rounded-full transition-all duration-300",
                activeIndex === index
                  ? "w-6 bg-primary"
                  : "w-2 bg-muted-foreground/30 hover:bg-muted-foreground/50"
              )}
              key={t._id}
              onClick={() => setActiveIndex(index)}
              type="button"
            />
          ))}
        </div>

        <div className="flex items-center gap-2">
          <Button
            aria-label="Previous testimonial"
            className="size-11 rounded-full"
            disabled={activeIndex === 0}
            onClick={prev}
            size="icon-xl"
            variant="outline"
          >
            <svg
              fill="none"
              height="16"
              stroke="currentColor"
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth="2"
              viewBox="0 0 24 24"
              width="16"
            >
              <path d="M19 12H5" />
              <path d="M12 19l-7-7 7-7" />
            </svg>
          </Button>
          <Button
            aria-label="Next testimonial"
            className="size-11 rounded-full"
            disabled={activeIndex === testimonials.length - 1}
            onClick={next}
            size="icon-xl"
            variant="outline"
          >
            <svg
              fill="none"
              height="16"
              stroke="currentColor"
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth="2"
              viewBox="0 0 24 24"
              width="16"
            >
              <path d="M5 12h14" />
              <path d="M12 5l7 7-7 7" />
            </svg>
          </Button>
        </div>
      </div>
    </div>
  );
};

export default TestimonialCarousel;
