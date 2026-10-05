import type { ProviderUsageHistoryPoint } from "@/lib/types/provider-usage";
import { clampPct } from "./usage-format";

const WIDTH = 160;
const HEIGHT = 32;
const PAD = 3;

type Props = {
  points: ProviderUsageHistoryPoint[];
};

function polyline(points: ProviderUsageHistoryPoint[], minT: number, span: number): string {
  return points
    .map((point) => {
      const x = PAD + ((new Date(point.at).getTime() - minT) / span) * (WIDTH - 2 * PAD);
      const y = HEIGHT - PAD - (clampPct(point.pct) / 100) * (HEIGHT - 2 * PAD);
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(" ");
}

/**
 * Inline SVG sparkline. Measured points are solid, estimated points dashed,
 * and limit hits are vertical markers; no chart dependency is added.
 */
export function UsageSparkline({ points }: Props) {
  if (points.length < 2) return null;
  const times = points.map((point) => new Date(point.at).getTime());
  const minT = Math.min(...times);
  const span = Math.max(...times) - minT || 1;
  const measured = points.filter((point) => point.kind === "measured");
  const estimated = points.filter((point) => point.kind === "estimated");
  const limitHits = points.filter((point) => point.kind === "limit_hit");

  return (
    <svg viewBox={`0 0 ${WIDTH} ${HEIGHT}`} className="h-8 w-full" aria-hidden="true">
      {measured.length > 1 && (
        <polyline
          points={polyline(measured, minT, span)}
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          className="text-blue-500"
        />
      )}
      {estimated.length > 1 && (
        <polyline
          points={polyline(estimated, minT, span)}
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeDasharray="3 3"
          className="text-muted-foreground"
        />
      )}
      {limitHits.map((point) => {
        const x = PAD + ((new Date(point.at).getTime() - minT) / span) * (WIDTH - 2 * PAD);
        return (
          <line
            key={point.at}
            x1={x}
            x2={x}
            y1={PAD}
            y2={HEIGHT - PAD}
            stroke="currentColor"
            strokeWidth="1"
            className="text-red-500"
          />
        );
      })}
    </svg>
  );
}
