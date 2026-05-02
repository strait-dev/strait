import { cn } from "@strait/ui/utils";
import { useReducedMotion } from "motion/react";
import { useEffect, useRef, useState } from "react";

type ParticlesProps = {
  className?: string;
  quantity?: number;
  staticity?: number;
  ease?: number;
  size?: number;
  color?: string;
  vx?: number;
  vy?: number;
};

type Circle = {
  x: number;
  y: number;
  translateX: number;
  translateY: number;
  size: number;
  alpha: number;
  targetAlpha: number;
  dx: number;
  dy: number;
  magnetism: number;
};

const Particles = ({
  className,
  quantity = 50,
  staticity = 50,
  ease = 50,
  size = 0.4,
  color = "#ffffff",
  vx = 0,
  vy = 0,
}: ParticlesProps) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const canvasContainerRef = useRef<HTMLDivElement>(null);
  const context = useRef<CanvasRenderingContext2D | null>(null);
  const circles = useRef<Circle[]>([]);
  const mouse = useRef({ x: 0, y: 0 });
  const canvasSize = useRef({ w: 0, h: 0 });
  const dprRef = useRef(1);
  const isVisibleRef = useRef(true);
  const prefersReducedMotion = useReducedMotion();
  const reducedMotionRef = useRef(prefersReducedMotion);
  reducedMotionRef.current = prefersReducedMotion;
  const [rgb, setRgb] = useState({ r: 255, g: 255, b: 255 });
  const rafRef = useRef(0);

  useEffect(() => {
    const tempEl = document.createElement("div");
    tempEl.style.color = color;
    document.body.appendChild(tempEl);
    const computed = getComputedStyle(tempEl).color;
    document.body.removeChild(tempEl);
    const match = computed.match(/\d+/g);
    if (match && match.length >= 3) {
      setRgb({
        r: Number.parseInt(match[0] ?? "255", 10),
        g: Number.parseInt(match[1] ?? "255", 10),
        b: Number.parseInt(match[2] ?? "255", 10),
      });
    }
  }, [color]);

  useEffect(() => {
    if (!(canvasRef.current && canvasContainerRef.current)) {
      return;
    }
    dprRef.current = window.devicePixelRatio || 1;
    context.current = canvasRef.current.getContext("2d");

    const dpr = dprRef.current;

    const createCircle = (): Circle => ({
      x: Math.random() * canvasSize.current.w,
      y: Math.random() * canvasSize.current.h,
      translateX: 0,
      translateY: 0,
      size: Math.random() * 2 + size,
      alpha: 0,
      targetAlpha: Math.random() * 0.6 + 0.1,
      dx: (Math.random() - 0.5) * 0.1,
      dy: (Math.random() - 0.5) * 0.1,
      magnetism: 0.1 + Math.random() * 4,
    });

    const initCanvas = () => {
      if (
        !(canvasContainerRef.current && canvasRef.current && context.current)
      ) {
        return;
      }
      const { offsetWidth, offsetHeight } = canvasContainerRef.current;
      canvasSize.current = { w: offsetWidth, h: offsetHeight };
      canvasRef.current.width = offsetWidth * dpr;
      canvasRef.current.height = offsetHeight * dpr;
      canvasRef.current.style.width = `${offsetWidth}px`;
      canvasRef.current.style.height = `${offsetHeight}px`;
      context.current.scale(dpr, dpr);
      circles.current = [];
      for (let i = 0; i < quantity; i++) {
        circles.current.push(createCircle());
      }
    };

    const drawCircle = (circle: Circle) => {
      if (!context.current) {
        return;
      }
      const { x, y, translateX, translateY, alpha } = circle;
      context.current.translate(translateX, translateY);
      context.current.beginPath();
      context.current.arc(x, y, circle.size, 0, 2 * Math.PI);
      context.current.fillStyle = `rgba(${rgb.r}, ${rgb.g}, ${rgb.b}, ${alpha})`;
      context.current.fill();
      context.current.setTransform(dpr, 0, 0, dpr, 0, 0);
    };

    const updateCircle = (circle: Circle) => {
      const edgeDistX = Math.min(circle.x, canvasSize.current.w - circle.x);
      const edgeDistY = Math.min(circle.y, canvasSize.current.h - circle.y);
      const clampedFade = Math.min(Math.min(edgeDistX, edgeDistY) / 40, 1);
      circle.alpha += (circle.targetAlpha * clampedFade - circle.alpha) * 0.08;

      circle.x += circle.dx + vx;
      circle.y += circle.dy + vy;
      circle.translateX +=
        (mouse.current.x / (staticity / circle.magnetism) - circle.translateX) /
        ease;
      circle.translateY +=
        (mouse.current.y / (staticity / circle.magnetism) - circle.translateY) /
        ease;

      const w = canvasSize.current.w;
      const h = canvasSize.current.h;
      if (circle.x < -10) {
        circle.x = w + 10;
      } else if (circle.x > w + 10) {
        circle.x = -10;
      }
      if (circle.y < -10) {
        circle.y = h + 10;
      } else if (circle.y > h + 10) {
        circle.y = -10;
      }
    };

    const animate = () => {
      rafRef.current = requestAnimationFrame(animate);
      if (
        !(isVisibleRef.current && context.current) ||
        reducedMotionRef.current
      ) {
        return;
      }
      context.current.clearRect(
        0,
        0,
        canvasSize.current.w,
        canvasSize.current.h
      );

      for (const circle of circles.current) {
        updateCircle(circle);
        drawCircle(circle);
      }
    };

    initCanvas();
    rafRef.current = requestAnimationFrame(animate);

    const resizeObserver = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (!entry) {
        return;
      }
      initCanvas();
    });
    resizeObserver.observe(canvasContainerRef.current);

    const intersectionObserver = new IntersectionObserver(
      ([entry]) => {
        isVisibleRef.current = entry?.isIntersecting ?? false;
      },
      { threshold: 0 }
    );
    intersectionObserver.observe(canvasContainerRef.current);

    return () => {
      cancelAnimationFrame(rafRef.current);
      resizeObserver.disconnect();
      intersectionObserver.disconnect();
    };
  }, [rgb, ease, quantity, size, staticity, vx, vy]);

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!canvasContainerRef.current) {
        return;
      }
      const rect = canvasContainerRef.current.getBoundingClientRect();
      mouse.current = {
        x: e.clientX - rect.left,
        y: e.clientY - rect.top,
      };
    };
    window.addEventListener("mousemove", handleMouseMove, { passive: true });
    return () => window.removeEventListener("mousemove", handleMouseMove);
  }, []);

  return (
    <div className={cn("absolute inset-0", className)} ref={canvasContainerRef}>
      <canvas
        className="pointer-events-none absolute inset-0"
        ref={canvasRef}
      />
    </div>
  );
};

export default Particles;
