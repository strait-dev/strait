import { cn } from "@strait/ui/utils";
import { useReducedMotion } from "motion/react";
import { useMemo } from "react";
import { createMap } from "svg-dotted-map";

type Marker = {
  lat: number;
  lng: number;
  label?: string;
};

type DottedMapProps = {
  className?: string;
  dotColor?: string;
  markerColor?: string;
  markers?: Marker[];
  backgroundColor?: string;
};

const MAP_WIDTH = 200;
const MAP_HEIGHT = 100;

const DottedMap = ({
  className,
  dotColor = "currentColor",
  markerColor = "var(--primary)",
  markers = [],
  backgroundColor = "transparent",
}: DottedMapProps) => {
  const prefersReducedMotion = useReducedMotion();

  const { dots, projectedMarkers } = useMemo(() => {
    const map = createMap({
      height: MAP_HEIGHT,
      width: MAP_WIDTH,
    });
    const points = map.points;
    const projected = map.addMarkers(
      markers.map((m) => ({ lat: m.lat, lng: m.lng }))
    );
    return { dots: points, projectedMarkers: projected };
  }, [markers]);

  return (
    <div
      className={cn("relative w-full overflow-hidden", className)}
      style={{ backgroundColor }}
    >
      <svg
        className="h-auto w-full"
        preserveAspectRatio="xMidYMid meet"
        viewBox={`0 0 ${MAP_WIDTH} ${MAP_HEIGHT}`}
      >
        {/* Map dots */}
        {dots.map((point) => (
          <circle
            cx={point.x}
            cy={point.y}
            fill={dotColor}
            key={`${point.x}-${point.y}`}
            opacity={0.4}
            r={0.4}
          />
        ))}
        {/* Markers */}
        {projectedMarkers.map((marker) => (
          <g key={`m-${marker.x}-${marker.y}`}>
            <circle cx={marker.x} cy={marker.y} fill={markerColor} r={1} />
            <circle
              cx={marker.x}
              cy={marker.y}
              fill={markerColor}
              opacity={0.3}
              r={2.5}
            >
              {!prefersReducedMotion && (
                <>
                  <animate
                    attributeName="r"
                    dur="2s"
                    repeatCount="indefinite"
                    values="1.5;4;1.5"
                  />
                  <animate
                    attributeName="opacity"
                    dur="2s"
                    repeatCount="indefinite"
                    values="0.4;0.1;0.4"
                  />
                </>
              )}
            </circle>
          </g>
        ))}
      </svg>
    </div>
  );
};

export default DottedMap;
