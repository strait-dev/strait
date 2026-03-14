import { Button } from "@strait/ui/components/button";
import { Link } from "@tanstack/react-router";

const NotFound = () => {
  return (
    <div className="space-y-2 p-2">
      <p>The page you are looking for does not exist.</p>
      <p className="flex flex-wrap items-center gap-2">
        <Button onClick={() => window.history.back()} type="button">
          Go back
        </Button>
        <Button render={<Link preload="intent" to="/" />} variant="secondary">
          Home
        </Button>
      </p>
    </div>
  );
};

export default NotFound;
