"use client";

import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";
import { useEffect, useRef } from "react";

import Shell from "@/components/layout/shell.tsx";
import { dashboardHref } from "@/lib/urls.ts";

type Particle = { x: number; y: number; vx: number; vy: number };

function updateParticles(particles: Particle[], w: number, h: number) {
  for (const p of particles) {
    p.x += p.vx;
    p.y += p.vy;
    if (p.x < 0 || p.x > w) {
      p.vx *= -1;
    }
    if (p.y < 0 || p.y > h) {
      p.vy *= -1;
    }
  }
}

function drawConnections(
  ctx: CanvasRenderingContext2D,
  particles: Particle[],
  maxDist: number
) {
  for (let i = 0; i < particles.length; i++) {
    for (let j = i + 1; j < particles.length; j++) {
      const a = particles[i];
      const b = particles[j];
      if (!(a && b)) {
        continue;
      }
      const dx = a.x - b.x;
      const dy = a.y - b.y;
      const dist = Math.sqrt(dx * dx + dy * dy);
      if (dist < maxDist) {
        ctx.strokeStyle = `rgba(255,255,255,${0.08 * (1 - dist / maxDist)})`;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(a.x, a.y);
        ctx.lineTo(b.x, b.y);
        ctx.stroke();
      }
    }
  }
}

const ConstellationBg = () => {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) {
      return;
    }
    const ctx = canvas.getContext("2d");
    if (!ctx) {
      return;
    }

    const particles: Particle[] = [];
    const PARTICLE_COUNT = 35;
    const CONNECTION_DISTANCE = 120;

    canvas.width = canvas.offsetWidth * 2;
    canvas.height = canvas.offsetHeight * 2;

    for (let i = 0; i < PARTICLE_COUNT; i++) {
      particles.push({
        x: Math.random() * canvas.width,
        y: Math.random() * canvas.height,
        vx: (Math.random() - 0.5) * 0.4,
        vy: (Math.random() - 0.5) * 0.4,
      });
    }

    let rafId = 0;
    const draw = () => {
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      updateParticles(particles, canvas.width, canvas.height);
      drawConnections(ctx, particles, CONNECTION_DISTANCE);

      for (const p of particles) {
        ctx.fillStyle = "rgba(255,255,255,0.15)";
        ctx.beginPath();
        ctx.arc(p.x, p.y, 2, 0, Math.PI * 2);
        ctx.fill();
      }

      rafId = requestAnimationFrame(draw);
    };

    draw();
    return () => cancelAnimationFrame(rafId);
  }, []);

  return (
    <canvas
      className="pointer-events-none absolute inset-0 h-full w-full"
      ref={canvasRef}
      style={{ opacity: 0.6 }}
    />
  );
};

const CTA = () => {
  const headingId = "cta-title";

  return (
    <section
      aria-labelledby={headingId}
      className="relative border-border/40 border-y bg-primary py-20 sm:py-28"
    >
      <div className="orchestration-grid pointer-events-none absolute inset-0 opacity-[0.12]" />
      <ConstellationBg />

      <Shell className="relative z-10" variant="wide">
        <div className="flex flex-col items-center text-center">
          <h2
            className="max-w-3xl text-2xl text-primary-foreground leading-[1.1] tracking-tighter sm:text-3xl lg:text-4xl"
            id={headingId}
          >
            Stop building job infrastructure. Start shipping workflows.
          </h2>

          <p className="mt-6 max-w-2xl text-base text-primary-foreground/70 leading-relaxed sm:text-lg">
            Deploy your first workflow in under 10 minutes. Zero-broker,
            production-grade from the first run.
          </p>

          <div className="mt-10 flex flex-col items-center gap-4">
            <Button
              render={<Link href={dashboardHref("/login")} />}
              variant="outline"
            >
              Deploy your first workflow
              <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
            </Button>
            <p className="text-primary-foreground/50 text-sm">
              No credit card required. Runs on your Postgres.
            </p>
          </div>
        </div>
      </Shell>
    </section>
  );
};

export default CTA;
