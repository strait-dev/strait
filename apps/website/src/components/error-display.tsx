import { Button } from "@strait/ui/components/button";
import { useEffect, useRef } from "react";

type ErrorDisplayProps = {
  code?: string;
  title: string;
  description: string;
  actions: Array<{
    label: string;
    href?: string;
    onClick?: () => void;
    variant?: "default" | "outline" | "ghost";
  }>;
};

const GRID_SIZE = 24;
const CELL = 28;

const FloatingGrid = () => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const rafRef = useRef(0);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) {
      return;
    }
    const ctx = canvas.getContext("2d");
    if (!ctx) {
      return;
    }

    const dpr = window.devicePixelRatio || 1;
    const width = GRID_SIZE * CELL;
    const height = GRID_SIZE * CELL;
    canvas.width = width * dpr;
    canvas.height = height * dpr;
    canvas.style.width = `${String(width)}px`;
    canvas.style.height = `${String(height)}px`;
    ctx.scale(dpr, dpr);

    const cells: number[] = Array.from(
      { length: GRID_SIZE * GRID_SIZE },
      () => Math.random() * 0.3
    );

    let time = 0;

    const animate = () => {
      time += 0.008;
      ctx.clearRect(0, 0, width, height);

      for (let i = 0; i < GRID_SIZE; i++) {
        for (let j = 0; j < GRID_SIZE; j++) {
          const idx = i * GRID_SIZE + j;
          const baseAlpha = cells[idx] ?? 0;
          const wave =
            Math.sin(time * 2 + i * 0.3 + j * 0.2) * 0.15 +
            Math.cos(time * 1.5 + j * 0.4) * 0.1;
          const alpha = Math.max(0, Math.min(baseAlpha + wave, 0.4));

          ctx.fillStyle = `rgba(255, 255, 255, ${String(alpha * 0.08)})`;
          ctx.fillRect(i * CELL + 1, j * CELL + 1, CELL - 2, CELL - 2);
        }
      }

      rafRef.current = requestAnimationFrame(animate);
    };

    rafRef.current = requestAnimationFrame(animate);
    return () => cancelAnimationFrame(rafRef.current);
  }, []);

  return (
    <canvas
      className="pointer-events-none absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 opacity-60"
      ref={canvasRef}
    />
  );
};

const ErrorDisplay = ({
  code,
  title,
  description,
  actions,
}: ErrorDisplayProps) => (
  <div className="relative flex min-h-[60vh] items-center justify-center overflow-hidden py-20 sm:py-28">
    <FloatingGrid />
    <div className="absolute inset-0 bg-[radial-gradient(ellipse_60%_50%_at_50%_50%,_transparent,_var(--background))]" />

    <div className="relative z-10 mx-auto max-w-lg px-4 text-center sm:px-6">
      {code && (
        <p className="font-mono text-7xl text-foreground/10 sm:text-9xl">
          {code}
        </p>
      )}
      <h1 className="mt-2 text-4xl text-foreground sm:text-5xl lg:text-6xl">
        {title}
      </h1>
      <p className="mt-4 text-pretty text-muted-foreground text-sm leading-relaxed sm:text-base">
        {description}
      </p>
      <div className="mt-8 flex items-center justify-center gap-3">
        {actions.map((action) =>
          action.href ? (
            <Button
              key={action.label}
              // biome-ignore lint/a11y/useAnchorContent: content provided by Button children
              render={<a href={action.href} />}
              size="default"
              variant={action.variant ?? "default"}
            >
              {action.label}
            </Button>
          ) : (
            <Button
              key={action.label}
              onClick={action.onClick}
              size="default"
              type="button"
              variant={action.variant ?? "default"}
            >
              {action.label}
            </Button>
          )
        )}
      </div>
    </div>
  </div>
);

export default ErrorDisplay;
