
import { useState } from "react";
import { Button } from "@strait/ui/components/button";
import { Separator } from "@strait/ui/components/separator";

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
      <div className="mt-8">
        <Separator />
        <p className="pt-6 text-muted-foreground text-sm">
          Thanks for your feedback!
        </p>
      </div>
    );
  }

  return (
    <div className="mt-8">
      <Separator />
      <div className="flex items-center gap-3 pt-6">
        <span className="text-muted-foreground text-sm">
          Was this page helpful?
        </span>
        <Button
          variant={rating === "up" ? "default" : "outline"}
          size="sm"
          onClick={() => handleRating("up")}
        >
          Yes
        </Button>
        <Button
          variant={rating === "down" ? "destructive" : "outline"}
          size="sm"
          onClick={() => handleRating("down")}
        >
          No
        </Button>
      </div>
    </div>
  );
}
