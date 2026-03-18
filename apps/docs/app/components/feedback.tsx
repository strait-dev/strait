"use client";

import { useState } from "react";

type Rating = "up" | "down" | null;

export function Feedback() {
  const [rating, setRating] = useState<Rating>(null);
  const [submitted, setSubmitted] = useState(false);

  const handleRating = (value: Rating) => {
    setRating(value);
    setSubmitted(true);
  };

  if (submitted) {
    return (
      <div className="mt-8 flex items-center gap-2 border-border border-t pt-6 text-muted-foreground text-sm">
        Thanks for your feedback!
      </div>
    );
  }

  return (
    <div className="mt-8 flex items-center gap-3 border-border border-t pt-6">
      <span className="text-muted-foreground text-sm">
        Was this page helpful?
      </span>
      <button
        type="button"
        onClick={() => handleRating("up")}
        className={`rounded-md border px-3 py-1 text-sm transition-colors ${
          rating === "up"
            ? "border-primary bg-primary/10 text-primary"
            : "border-border hover:bg-accent"
        }`}
      >
        Yes
      </button>
      <button
        type="button"
        onClick={() => handleRating("down")}
        className={`rounded-md border px-3 py-1 text-sm transition-colors ${
          rating === "down"
            ? "border-destructive bg-destructive/10 text-destructive"
            : "border-border hover:bg-accent"
        }`}
      >
        No
      </button>
    </div>
  );
}
