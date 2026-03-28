
import { cn } from "@strait/ui/utils";
import Autoplay from "embla-carousel-autoplay";
import useEmblaCarousel from "embla-carousel-react";
import { useCallback, useEffect, useState } from "react";

type Slide = {
  number: number;
  name: string;
  description: string;
};

type LoadingCarouselProps = {
  slides: Slide[];
  className?: string;
  autoplayDelay?: number;
};

const LoadingCarousel = ({
  slides,
  className,
  autoplayDelay = 4000,
}: LoadingCarouselProps) => {
  const [emblaRef, emblaApi] = useEmblaCarousel({ loop: true }, [
    Autoplay({ delay: autoplayDelay, stopOnInteraction: false }),
  ]);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [progress, setProgress] = useState(0);

  const onSelect = useCallback(() => {
    if (!emblaApi) {
      return;
    }
    setSelectedIndex(emblaApi.selectedScrollSnap());
    setProgress(0);
  }, [emblaApi]);

  useEffect(() => {
    if (!emblaApi) {
      return;
    }
    emblaApi.on("select", onSelect);
    onSelect();
    return () => {
      emblaApi.off("select", onSelect);
    };
  }, [emblaApi, onSelect]);

  useEffect(() => {
    const interval = setInterval(() => {
      setProgress((prev) => {
        if (prev >= 100) {
          return 0;
        }
        return prev + 100 / (autoplayDelay / 50);
      });
    }, 50);
    return () => clearInterval(interval);
  }, [autoplayDelay]);

  return (
    <div className={cn("w-full", className)}>
      <div className="overflow-hidden" ref={emblaRef}>
        <div className="flex">
          {slides.map((slide) => (
            <div className="min-w-0 flex-[0_0_100%] px-2" key={slide.number}>
              <div className="rounded-xl border border-border/60 bg-card p-6 sm:p-8">
                <div className="flex items-center gap-4">
                  <div className="flex size-10 shrink-0 items-center justify-center rounded-full border border-primary/30 bg-primary/10 font-medium text-primary text-sm">
                    {slide.number}
                  </div>
                  <div>
                    <p className="font-medium text-foreground">{slide.name}</p>
                    <p className="mt-1 text-muted-foreground text-sm leading-relaxed">
                      {slide.description}
                    </p>
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Progress indicators */}
      <div className="mt-4 flex items-center justify-center gap-2">
        {slides.map((slide, i) => {
          let width = "0%";
          if (i === selectedIndex) {
            width = `${progress}%`;
          } else if (i < selectedIndex) {
            width = "100%";
          }
          return (
            <button
              className="relative h-1.5 max-w-12 flex-1 overflow-hidden rounded-full bg-border/40"
              key={slide.number}
              onClick={() => emblaApi?.scrollTo(i)}
              type="button"
            >
              <div
                className="absolute inset-y-0 left-0 rounded-full bg-primary transition-all duration-100"
                style={{ width }}
              />
            </button>
          );
        })}
      </div>
    </div>
  );
};

export default LoadingCarousel;
