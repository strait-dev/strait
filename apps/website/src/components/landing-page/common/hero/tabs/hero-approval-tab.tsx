import { useEffect, useRef, useState } from "react";

type StepStatus = "completed" | "executing" | "approval" | "queued";

type Step = {
  id: string;
  label: string;
  status: StepStatus;
};

const INITIAL_STEPS: Step[] = [
  { id: "validate", label: "Validate Order", status: "queued" },
  { id: "charge", label: "Charge Payment", status: "queued" },
  { id: "review", label: "Manager Review", status: "queued" },
  { id: "fulfill", label: "Fulfill Order", status: "queued" },
];

type Event = {
  delay: number;
  stepId: string;
  status: StepStatus;
  detail?: string;
};

const EVENTS: Event[] = [
  { delay: 0, stepId: "validate", status: "executing" },
  { delay: 700, stepId: "validate", status: "completed" },
  { delay: 1000, stepId: "charge", status: "executing" },
  { delay: 1800, stepId: "charge", status: "completed" },
  {
    delay: 2100,
    stepId: "review",
    status: "approval",
    detail: "Awaiting manager approval",
  },
  {
    delay: 4600,
    stepId: "review",
    status: "completed",
    detail: "Approved by sarah@company.io",
  },
  { delay: 4900, stepId: "fulfill", status: "executing" },
  { delay: 5700, stepId: "fulfill", status: "completed" },
];

const CYCLE_DURATION = 7500;

const statusClass: Record<StepStatus, string> = {
  queued: "border-border/50 bg-muted/20 text-muted-foreground/50",
  executing: "border-primary/40 bg-primary/8 text-primary",
  approval: "border-warning/40 bg-warning/8 text-warning",
  completed: "border-success/40 bg-success/8 text-success",
};

const statusIcon: Record<StepStatus, React.ReactNode> = {
  queued: <div className="size-4 rounded-full border border-border/60" />,
  executing: (
    <div className="size-4 rounded-full border-2 border-primary bg-primary/20">
      <div className="size-full animate-pulse rounded-full bg-primary/40" />
    </div>
  ),
  approval: (
    <svg
      className="size-4 text-warning"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      viewBox="0 0 24 24"
    >
      <path
        d="M12 9v3m0 3h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  ),
  completed: (
    <svg
      className="size-4 text-success"
      fill="none"
      stroke="currentColor"
      strokeWidth={2.5}
      viewBox="0 0 24 24"
    >
      <path d="M5 13l4 4L19 7" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  ),
};

const HeroApprovalTab = () => {
  const [steps, setSteps] = useState<Step[]>(INITIAL_STEPS);
  const [detail, setDetail] = useState<string | null>(null);
  const rafRef = useRef<number>(0);

  useEffect(() => {
    let startTime: number | null = null;
    let lastApplied = -1;

    const tick = (timestamp: number) => {
      if (startTime === null) {
        startTime = timestamp;
      }
      const elapsed = (timestamp - startTime) % CYCLE_DURATION;

      if (elapsed < 100 && lastApplied !== -1) {
        setSteps(INITIAL_STEPS);
        setDetail(null);
        lastApplied = -1;
      }

      for (let i = 0; i < EVENTS.length; i++) {
        const evt = EVENTS[i];
        if (evt && elapsed >= evt.delay && i > lastApplied) {
          lastApplied = i;
          setSteps((prev) =>
            prev.map((s) =>
              s.id === evt.stepId ? { ...s, status: evt.status } : s
            )
          );
          if (evt.detail !== undefined) {
            setDetail(evt.detail);
          }
        }
      }

      rafRef.current = requestAnimationFrame(tick);
    };

    rafRef.current = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(rafRef.current);
  }, []);

  const approvalStep = steps.find((s) => s.id === "review");
  const isWaiting = approvalStep?.status === "approval";
  const isApproved = approvalStep?.status === "completed";

  return (
    <div className="flex h-full flex-col">
      {/* Workflow steps */}
      <div className="flex-1 px-5 py-5 sm:px-6 sm:py-6">
        <div className="mb-4 flex items-center gap-2">
          <span className="font-mono text-[11px] text-muted-foreground/50">
            workflow
          </span>
          <span className="font-mono text-foreground text-xs">
            checkout-flow
          </span>
          <span className="ml-2 font-mono text-[11px] text-muted-foreground/50">
            run_id
          </span>
          <span className="font-mono text-foreground text-xs">run_m8k21v</span>
        </div>

        <div className="space-y-2">
          {steps.map((step, idx) => (
            <div key={step.id}>
              <div
                className={`flex items-center gap-3 rounded-lg border px-3.5 py-2.5 transition-all duration-300 ${statusClass[step.status]}`}
              >
                {statusIcon[step.status]}
                <span className="font-medium text-xs">{step.label}</span>
                {step.status === "completed" && (
                  <span className="ml-auto font-mono text-[10px] opacity-50">
                    {["215ms", "1.07s", "2.5s", "685ms"][idx]}
                  </span>
                )}
                {step.status === "approval" && (
                  <span className="ml-auto font-mono text-[10px]">
                    waiting...
                  </span>
                )}
              </div>
              {/* Connector */}
              {idx < steps.length - 1 && (
                <div className="ml-5 h-2 w-px bg-border/40" />
              )}
            </div>
          ))}
        </div>
      </div>

      {/* Approval banner */}
      <div className="border-border/40 border-t bg-muted/20 px-5 py-3 sm:px-6">
        {isWaiting && detail && (
          <div
            className="flex animate-fade-in-up items-center gap-2"
            style={{ animationDuration: "200ms" }}
          >
            <span className="size-2 animate-pulse rounded-full bg-warning" />
            <span className="font-medium text-warning text-xs">{detail}</span>
          </div>
        )}
        {isApproved && detail && (
          <div
            className="flex animate-fade-in-up items-center gap-2"
            style={{ animationDuration: "200ms" }}
          >
            <svg
              className="size-3.5 text-success"
              fill="none"
              stroke="currentColor"
              strokeWidth={2.5}
              viewBox="0 0 24 24"
            >
              <path
                d="M5 13l4 4L19 7"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
            <span className="font-medium text-success text-xs">{detail}</span>
          </div>
        )}
        {!(isWaiting || isApproved) && (
          <span className="text-muted-foreground/30 text-xs">
            No pending approvals
          </span>
        )}
      </div>
    </div>
  );
};

export default HeroApprovalTab;
