const BLOBS = [
  {
    className:
      "absolute -top-1/4 -left-1/4 h-[60%] w-[60%] rounded-full opacity-20 blur-3xl",
    style: {
      background: "color-mix(in oklch, var(--primary) 80%, oklch(0.7 0.15 90))",
      animation: "mesh-a 14s ease-in-out infinite",
    },
  },
  {
    className:
      "absolute top-1/3 -right-1/4 h-[50%] w-[50%] rounded-full opacity-15 blur-3xl",
    style: {
      background:
        "color-mix(in oklch, var(--primary) 70%, oklch(0.7 0.15 130))",
      animation: "mesh-b 18s ease-in-out infinite",
    },
  },
  {
    className:
      "absolute -bottom-1/4 left-1/4 h-[55%] w-[55%] rounded-full opacity-15 blur-3xl",
    style: {
      background: "color-mix(in oklch, var(--primary) 60%, oklch(0.8 0.12 80))",
      animation: "mesh-c 16s ease-in-out infinite",
    },
  },
  {
    className:
      "absolute top-0 right-1/3 h-[40%] w-[40%] rounded-full opacity-10 blur-3xl",
    style: {
      background:
        "color-mix(in oklch, var(--primary) 90%, oklch(0.6 0.18 120))",
      animation: "mesh-d 20s ease-in-out infinite",
    },
  },
];

const MeshGradientBg = () => (
  <>
    <div className="pointer-events-none absolute inset-0 overflow-hidden">
      {BLOBS.map((blob, i) => (
        <div
          className={blob.className}
          key={`mesh-${String(i)}`}
          style={blob.style}
        />
      ))}
    </div>
    <style>{`
      @keyframes mesh-a {
        0%, 100% { transform: translate(0, 0); }
        33% { transform: translate(15%, 10%); }
        66% { transform: translate(-5%, 15%); }
      }
      @keyframes mesh-b {
        0%, 100% { transform: translate(0, 0); }
        33% { transform: translate(-10%, -15%); }
        66% { transform: translate(10%, -5%); }
      }
      @keyframes mesh-c {
        0%, 100% { transform: translate(0, 0); }
        33% { transform: translate(10%, -10%); }
        66% { transform: translate(-15%, 5%); }
      }
      @keyframes mesh-d {
        0%, 100% { transform: translate(0, 0); }
        33% { transform: translate(-8%, 12%); }
        66% { transform: translate(12%, -8%); }
      }
      @media (prefers-reduced-motion: reduce) {
        [style*="mesh-"] {
          animation: none !important;
        }
      }
    `}</style>
  </>
);

export default MeshGradientBg;
