/* Static SVG illustrations for the feature bento grid.
   Each uses CSS custom properties for theme-aware colors. */

/* -- 1. Job Orchestration: State machine flow -- */
export const QueueAnimation = () => (
  <svg
    className="w-full max-w-[280px]"
    fill="none"
    viewBox="0 0 280 100"
    xmlns="http://www.w3.org/2000/svg"
  >
    <defs>
      <marker
        id="arrow"
        markerHeight="6"
        markerWidth="6"
        orient="auto-start-reverse"
        refX="5"
        refY="3"
        viewBox="0 0 6 6"
      >
        <path
          d="M0,0 L6,3 L0,6 Z"
          fill="var(--muted-foreground)"
          opacity="0.5"
        />
      </marker>
    </defs>
    {/* Nodes */}
    <rect fill="var(--muted)" height="32" rx="6" width="56" x="0" y="34" />
    <text
      dominantBaseline="central"
      fill="var(--muted-foreground)"
      fontSize="9"
      textAnchor="middle"
      x="28"
      y="50"
    >
      Queued
    </text>

    <rect
      fill="color-mix(in oklch, var(--primary) 15%, transparent)"
      height="32"
      rx="6"
      stroke="var(--primary)"
      strokeWidth="1"
      width="64"
      x="76"
      y="34"
    />
    <text
      dominantBaseline="central"
      fill="var(--primary)"
      fontSize="9"
      fontWeight="500"
      textAnchor="middle"
      x="108"
      y="50"
    >
      Executing
    </text>

    <rect
      fill="color-mix(in oklch, var(--success) 15%, transparent)"
      height="32"
      rx="6"
      stroke="var(--success)"
      strokeWidth="1"
      width="68"
      x="160"
      y="10"
    />
    <text
      dominantBaseline="central"
      fill="var(--success)"
      fontSize="9"
      textAnchor="middle"
      x="194"
      y="26"
    >
      Completed
    </text>

    <rect
      fill="color-mix(in oklch, var(--destructive) 15%, transparent)"
      height="32"
      rx="6"
      stroke="var(--destructive)"
      strokeWidth="1"
      width="52"
      x="160"
      y="58"
    />
    <text
      dominantBaseline="central"
      fill="var(--destructive)"
      fontSize="9"
      textAnchor="middle"
      x="186"
      y="74"
    >
      Failed
    </text>

    <rect fill="var(--muted)" height="32" rx="6" width="40" x="232" y="58" />
    <text
      dominantBaseline="central"
      fill="var(--muted-foreground)"
      fontSize="9"
      textAnchor="middle"
      x="252"
      y="74"
    >
      DLQ
    </text>

    {/* Arrows */}
    <line
      markerEnd="url(#arrow)"
      stroke="var(--muted-foreground)"
      strokeOpacity="0.4"
      x1="56"
      x2="74"
      y1="50"
      y2="50"
    />
    <line
      markerEnd="url(#arrow)"
      stroke="var(--success)"
      strokeOpacity="0.5"
      x1="140"
      x2="158"
      y1="42"
      y2="30"
    />
    <line
      markerEnd="url(#arrow)"
      stroke="var(--destructive)"
      strokeOpacity="0.5"
      x1="140"
      x2="158"
      y1="58"
      y2="70"
    />
    <line
      markerEnd="url(#arrow)"
      stroke="var(--muted-foreground)"
      strokeOpacity="0.4"
      x1="212"
      x2="230"
      y1="74"
      y2="74"
    />
  </svg>
);

/* -- 2. Managed Execution: Region circles -- */
export const ExecutionAnimation = () => (
  <svg
    className="w-full max-w-[220px]"
    fill="none"
    viewBox="0 0 220 100"
    xmlns="http://www.w3.org/2000/svg"
  >
    {/* Dashed connections */}
    <line
      stroke="var(--border)"
      strokeDasharray="3 3"
      strokeWidth="1"
      x1="50"
      x2="110"
      y1="40"
      y2="40"
    />
    <line
      stroke="var(--border)"
      strokeDasharray="3 3"
      strokeWidth="1"
      x1="150"
      x2="110"
      y1="40"
      y2="40"
    />
    <line
      stroke="var(--border)"
      strokeDasharray="3 3"
      strokeWidth="1"
      x1="50"
      x2="100"
      y1="40"
      y2="80"
    />
    <line
      stroke="var(--border)"
      strokeDasharray="3 3"
      strokeWidth="1"
      x1="150"
      x2="100"
      y1="40"
      y2="80"
    />

    {/* iad - active */}
    <circle
      cx="50"
      cy="40"
      fill="color-mix(in oklch, var(--primary) 15%, transparent)"
      r="20"
      stroke="var(--primary)"
      strokeWidth="1.5"
    />
    <text
      dominantBaseline="central"
      fill="var(--primary)"
      fontSize="11"
      fontWeight="600"
      textAnchor="middle"
      x="50"
      y="38"
    >
      iad
    </text>
    <text
      dominantBaseline="central"
      fill="var(--primary)"
      fontSize="7"
      opacity="0.7"
      textAnchor="middle"
      x="50"
      y="50"
    >
      active
    </text>

    {/* lhr - warm */}
    <circle
      cx="150"
      cy="40"
      fill="var(--muted)"
      r="20"
      stroke="var(--border)"
      strokeWidth="1"
    />
    <text
      dominantBaseline="central"
      fill="var(--muted-foreground)"
      fontSize="11"
      fontWeight="500"
      textAnchor="middle"
      x="150"
      y="38"
    >
      lhr
    </text>
    <text
      dominantBaseline="central"
      fill="var(--muted-foreground)"
      fontSize="7"
      opacity="0.5"
      textAnchor="middle"
      x="150"
      y="50"
    >
      warm
    </text>

    {/* nrt - warm */}
    <circle
      cx="100"
      cy="80"
      fill="var(--muted)"
      r="20"
      stroke="var(--border)"
      strokeWidth="1"
    />
    <text
      dominantBaseline="central"
      fill="var(--muted-foreground)"
      fontSize="11"
      fontWeight="500"
      textAnchor="middle"
      x="100"
      y="78"
    >
      nrt
    </text>
    <text
      dominantBaseline="central"
      fill="var(--muted-foreground)"
      fontSize="7"
      opacity="0.5"
      textAnchor="middle"
      x="100"
      y="90"
    >
      warm
    </text>
  </svg>
);

/* -- 3. AI Agent Platform: Cost gauge -- */
export const CostAnimation = () => {
  const radius = 36;
  const circumference = Math.PI * radius;
  const progress = 0.65;
  const offset = circumference - progress * circumference;

  return (
    <svg
      className="w-full max-w-[160px]"
      fill="none"
      viewBox="0 0 160 100"
      xmlns="http://www.w3.org/2000/svg"
    >
      {/* Track */}
      <path
        d={`M ${80 - radius} 70 A ${radius} ${radius} 0 1 1 ${80 + radius} 70`}
        stroke="var(--muted)"
        strokeLinecap="round"
        strokeWidth="6"
      />
      {/* Fill */}
      <path
        d={`M ${80 - radius} 70 A ${radius} ${radius} 0 1 1 ${80 + radius} 70`}
        stroke="var(--primary)"
        strokeDasharray={`${circumference}`}
        strokeDashoffset={`${offset}`}
        strokeLinecap="round"
        strokeWidth="6"
      />
      {/* Budget line */}
      <line
        stroke="var(--destructive)"
        strokeDasharray="2 2"
        strokeOpacity="0.5"
        strokeWidth="1"
        x1="30"
        x2="130"
        y1="32"
        y2="32"
      />
      <text
        fill="var(--destructive)"
        fontSize="7"
        opacity="0.6"
        textAnchor="end"
        x="130"
        y="28"
      >
        budget
      </text>

      {/* Center text */}
      <text
        dominantBaseline="central"
        fill="var(--foreground)"
        fontSize="18"
        fontWeight="600"
        textAnchor="middle"
        x="80"
        y="52"
      >
        $7.80
      </text>
      <text
        dominantBaseline="central"
        fill="var(--muted-foreground)"
        fontSize="8"
        textAnchor="middle"
        x="80"
        y="68"
      >
        of $12 budget
      </text>
    </svg>
  );
};

/* -- 4. Language SDKs: Language badges -- */
export const SdkAnimation = () => {
  const langs = [
    { label: "TS", active: true },
    { label: "Py", active: false },
    { label: "Go", active: false },
    { label: "Rb", active: false },
    { label: "Rs", active: false },
  ];

  return (
    <svg
      className="w-full max-w-[240px]"
      fill="none"
      viewBox="0 0 240 52"
      xmlns="http://www.w3.org/2000/svg"
    >
      {langs.map((lang, i) => {
        const x = i * 46 + 7;
        return (
          <g key={lang.label}>
            <rect
              fill={
                lang.active
                  ? "color-mix(in oklch, var(--primary) 15%, transparent)"
                  : "var(--muted)"
              }
              height="36"
              rx="8"
              stroke={lang.active ? "var(--primary)" : "var(--border)"}
              strokeWidth={lang.active ? 1.5 : 1}
              width="36"
              x={x}
              y="8"
            />
            <text
              dominantBaseline="central"
              fill={lang.active ? "var(--primary)" : "var(--muted-foreground)"}
              fontSize="12"
              fontWeight={lang.active ? 600 : 400}
              textAnchor="middle"
              x={x + 18}
              y="26"
            >
              {lang.label}
            </text>
          </g>
        );
      })}
    </svg>
  );
};

/* -- 5. Built-in Observability: Health dashboard -- */
export const HealthAnimation = () => {
  const bars = [
    { label: "Queue", value: 92 },
    { label: "Workers", value: 88 },
    { label: "Latency", value: 96 },
  ];

  return (
    <svg
      className="w-full max-w-[220px]"
      fill="none"
      viewBox="0 0 220 110"
      xmlns="http://www.w3.org/2000/svg"
    >
      {/* Metric bars */}
      {bars.map((bar, i) => {
        const y = i * 26 + 8;
        return (
          <g key={bar.label}>
            <text
              dominantBaseline="central"
              fill="var(--muted-foreground)"
              fontSize="9"
              x="0"
              y={y + 6}
            >
              {bar.label}
            </text>
            {/* Track */}
            <rect
              fill="var(--muted)"
              height="8"
              rx="4"
              width="140"
              x="55"
              y={y + 1}
            />
            {/* Fill */}
            <rect
              fill="var(--primary)"
              height="8"
              opacity="0.6"
              rx="4"
              width={140 * (bar.value / 100)}
              x="55"
              y={y + 1}
            />
            <text
              dominantBaseline="central"
              fill="var(--muted-foreground)"
              fontSize="8"
              opacity="0.5"
              textAnchor="end"
              x="215"
              y={y + 6}
            >
              {bar.value}%
            </text>
          </g>
        );
      })}

      {/* Health score */}
      <text
        dominantBaseline="central"
        fill="var(--foreground)"
        fontSize="28"
        fontWeight="600"
        textAnchor="middle"
        x="110"
        y="98"
      >
        94
      </text>
      <text
        dominantBaseline="central"
        fill="var(--muted-foreground)"
        fontSize="10"
        textAnchor="start"
        x="126"
        y="98"
      >
        /100
      </text>
    </svg>
  );
};
