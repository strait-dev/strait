"use client";

import { cn } from "@strait/ui/utils";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

type FlickeringGridProps = {
  squareSize?: number;
  gridGap?: number;
  flickerChance?: number;
  color?: string;
  maxOpacity?: number;
  className?: string;
};

const FlickeringGrid = ({
  squareSize = 4,
  gridGap = 6,
  flickerChance = 0.3,
  color = "rgb(0, 0, 0)",
  maxOpacity = 0.3,
  className,
}: FlickeringGridProps) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [dimensions, setDimensions] = useState({ width: 0, height: 0 });

  const memoizedColor = useMemo(() => {
    const tempEl = document.createElement("div");
    tempEl.style.color = color;
    document.body.appendChild(tempEl);
    const computed = getComputedStyle(tempEl).color;
    document.body.removeChild(tempEl);
    const match = computed.match(/\d+/g);
    if (match && match.length >= 3) {
      return `${match[0]}, ${match[1]}, ${match[2]}`;
    }
    return "0, 0, 0";
  }, [color]);

  const setupCanvas = useCallback(
    (canvas: HTMLCanvasElement, width: number, height: number) => {
      const dpr = window.devicePixelRatio || 1;
      canvas.width = width * dpr;
      canvas.height = height * dpr;
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      const ctx = canvas.getContext("2d");
      if (!ctx) {
        return;
      }
      ctx.scale(dpr, dpr);
    },
    []
  );

  const drawGrid = useCallback(
    (
      ctx: CanvasRenderingContext2D,
      width: number,
      height: number,
      opacities: number[][]
    ) => {
      ctx.clearRect(0, 0, width, height);
      const cols = Math.ceil(width / (squareSize + gridGap));
      const rows = Math.ceil(height / (squareSize + gridGap));

      for (let i = 0; i < cols; i++) {
        for (let j = 0; j < rows; j++) {
          const opacity = opacities[i]?.[j] ?? 0;
          ctx.fillStyle = `rgba(${memoizedColor}, ${opacity})`;
          ctx.fillRect(
            i * (squareSize + gridGap),
            j * (squareSize + gridGap),
            squareSize,
            squareSize
          );
        }
      }
    },
    [squareSize, gridGap, memoizedColor]
  );

  useEffect(() => {
    const container = containerRef.current;
    if (!container) {
      return;
    }
    const obs = new ResizeObserver(([entry]) => {
      if (entry) {
        setDimensions({
          width: entry.contentRect.width,
          height: entry.contentRect.height,
        });
      }
    });
    obs.observe(container);
    return () => obs.disconnect();
  }, []);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || dimensions.width === 0) {
      return;
    }

    setupCanvas(canvas, dimensions.width, dimensions.height);
    const ctx = canvas.getContext("2d");
    if (!ctx) {
      return;
    }

    const cols = Math.ceil(dimensions.width / (squareSize + gridGap));
    const rows = Math.ceil(dimensions.height / (squareSize + gridGap));
    const opacities: number[][] = Array.from({ length: cols }, () =>
      Array.from({ length: rows }, () => Math.random() * maxOpacity)
    );

    let raf: number;
    const animate = () => {
      for (let i = 0; i < cols; i++) {
        for (let j = 0; j < rows; j++) {
          if (Math.random() < flickerChance * 0.016) {
            const row = opacities[i];
            if (row) {
              row[j] = Math.random() * maxOpacity;
            }
          }
        }
      }
      drawGrid(ctx, dimensions.width, dimensions.height, opacities);
      raf = requestAnimationFrame(animate);
    };
    raf = requestAnimationFrame(animate);
    return () => cancelAnimationFrame(raf);
  }, [
    dimensions,
    squareSize,
    gridGap,
    flickerChance,
    maxOpacity,
    setupCanvas,
    drawGrid,
  ]);

  return (
    <div className={cn("absolute inset-0", className)} ref={containerRef}>
      <canvas
        className="pointer-events-none absolute inset-0"
        ref={canvasRef}
      />
    </div>
  );
};

export default FlickeringGrid;
