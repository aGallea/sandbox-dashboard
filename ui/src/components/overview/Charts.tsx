import {
  Bar,
  BarChart,
  Cell,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import {
  OTHER_KEY,
  formatBytes,
  formatCores,
  type Bucket,
  type Slice,
} from '../../overview/aggregate';

import { AXIS, GRID, HOVER, tooltipStyle } from '../../viz/palette';

/**
 * When a chart is also a control, the reader needs to see which slice they
 * picked. Non-selected slices fade rather than recolour, so a hue still
 * identifies the group it has always identified.
 */
export interface Selectable {
  /** The slice currently scoping the page, if any. */
  selected?: string;
  /** Absent for a chart that only reports. `Other` is never offered. */
  onSelect?: (key: string) => void;
}

const dimmed = (s: Slice, selected?: string) =>
  selected && s.key !== selected ? 0.25 : 1;

/**
 * Part-to-whole for one dimension. The legend carries the label, count and
 * share for every slice, so no value is reachable only by hovering — which is
 * also the relief the low-contrast fills need on a white surface.
 */
export function Donut({
  slices,
  total,
  selected,
  onSelect,
}: { slices: Slice[]; total: number } & Selectable) {
  if (!total) return <p className="py-8 text-center text-sm text-slate-500">No sandboxes.</p>;

  // One category is not a chart. Say the number instead of drawing a ring
  // around the whole fleet.
  if (slices.length === 1) {
    return (
      <div className="flex flex-col items-center justify-center gap-2 py-8 text-center">
        <span className="text-5xl text-slate-900">{slices[0].count}</span>
        <span className="text-sm text-slate-600">
          all {slices[0].key.toLowerCase()} — one state across every sandbox shown
        </span>
      </div>
    );
  }

  return (
    <div className="flex flex-wrap items-center gap-x-6 gap-y-4">
      <div className="h-[168px] w-[168px] shrink-0">
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie
              data={slices}
              dataKey="count"
              nameKey="key"
              innerRadius="62%"
              outerRadius="100%"
              paddingAngle={2}
              stroke="#ffffff"
              strokeWidth={2}
              isAnimationActive={false}
            >
              {slices.map((s) => (
                <Cell
                  key={s.key}
                  fill={s.color}
                  fillOpacity={dimmed(s, selected)}
                  cursor={onSelect && s.key !== OTHER_KEY ? 'pointer' : undefined}
                  onClick={
                    onSelect && s.key !== OTHER_KEY ? () => onSelect(s.key) : undefined
                  }
                />
              ))}
            </Pie>
            <Tooltip
              contentStyle={tooltipStyle}
              formatter={(value: number, name: string) => [
                `${value} (${Math.round((value / total) * 100)}%)`,
                name,
              ]}
            />
          </PieChart>
        </ResponsiveContainer>
      </div>
      <dl className="min-w-[10rem] flex-1 space-y-1.5 text-sm">
        {slices.map((s) => {
          const pickable = !!onSelect && s.key !== OTHER_KEY;
          const row = (
            <>
              <span
                aria-hidden
                className="mt-1.5 h-2 w-2 shrink-0 rounded-full"
                style={{ background: s.color, opacity: dimmed(s, selected) }}
              />
              <dt
                className={`min-w-0 flex-1 truncate text-left ${
                  s.key === selected ? 'font-medium text-slate-900' : 'text-slate-600'
                }`}
                title={s.key}
              >
                {s.key}
              </dt>
              <dd className="tabular-nums text-slate-900">{s.count}</dd>
              <dd className="w-9 text-right tabular-nums text-slate-400">
                {Math.round((s.count / total) * 100)}%
              </dd>
            </>
          );
          return pickable ? (
            <button
              key={s.key}
              type="button"
              onClick={() => onSelect(s.key)}
              aria-pressed={s.key === selected}
              className="flex w-full items-baseline gap-2 rounded px-1 -mx-1 hover:bg-slate-50"
            >
              {row}
            </button>
          ) : (
            <div key={s.key} className="flex items-baseline gap-2 px-1 -mx-1">
              {row}
            </div>
          );
        })}
      </dl>
    </div>
  );
}

/**
 * One ratio against its reservation. The track is what the cluster set aside,
 * the fill is what the fleet is really using, and the gap between them is the
 * number worth reading — so it is spelled out rather than left to be inferred.
 */
export function Meter({
  label,
  reserved,
  used,
  format,
}: {
  label: string;
  reserved: number;
  used: number;
  format: (v: number) => string;
}) {
  const share = reserved > 0 ? Math.min(1, used / reserved) : 0;
  const percent = reserved > 0 ? used / reserved : 0;
  const shown = percent > 0 && percent < 0.01 ? '<1' : Math.round(percent * 100).toString();

  return (
    <div>
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-sm text-slate-600">{label}</span>
        <span className="text-sm tabular-nums text-slate-500">
          <strong className="font-semibold text-slate-900">{format(used)}</strong> of{' '}
          {format(reserved)} reserved
        </span>
      </div>
      <div className="mt-2 h-2.5 w-full overflow-hidden rounded-full bg-slate-100">
        <div
          className="h-full rounded-full bg-[#2a78d6]"
          style={{ width: `${Math.max(share * 100, share > 0 ? 0.75 : 0)}%` }}
        />
      </div>
      <p className="mt-1.5 text-xs text-slate-500">
        <span className="tabular-nums">{shown}%</span> in use ·{' '}
        <span className="tabular-nums">{format(Math.max(0, reserved - used))}</span> idle
      </p>
    </div>
  );
}

/** Age distribution. Ordered buckets, so one hue deepens with age. */
export function AgeHistogram({ buckets }: { buckets: Bucket[] }) {
  return (
    <div className="h-[196px]">
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={buckets} margin={{ top: 16, right: 8, bottom: 0, left: 0 }}>
          <XAxis
            dataKey="label"
            stroke={AXIS}
            fontSize={11}
            tickLine={false}
            axisLine={{ stroke: GRID }}
          />
          <YAxis
            stroke={AXIS}
            fontSize={11}
            tickLine={false}
            axisLine={false}
            width={32}
            allowDecimals={false}
          />
          <Tooltip
            contentStyle={tooltipStyle}
            formatter={(v: number) => [`${v} sandboxes`, 'up for']}
            cursor={{ fill: HOVER }}
          />
          <Bar dataKey="count" radius={[4, 4, 0, 0]} isAnimationActive={false} label={countLabel}>
            {buckets.map((b) => (
              <Cell key={b.label} fill={b.color} />
            ))}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

const countLabel = {
  position: 'top' as const,
  fontSize: 11,
  fill: '#52525b',
  formatter: (v: number) => (v ? v : ''),
};

/**
 * Reserved against used, per group, biggest reservation first.
 *
 * The bar is what the group holds; the darker inset is what it is really using.
 * That comparison is the point — a ranking of reservations alone would only
 * repeat what the share view above already says.
 */
export function FootprintBars({ slices, showGpu }: { slices: Slice[]; showGpu: boolean }) {
  const rows = [...slices].sort((a, b) => b.cpuMillis - a.cpuMillis);
  const widest = Math.max(...rows.map((r) => r.cpuMillis), 1);
  const haveUsage = rows.some((r) => r.usedCores > 0);
  if (!rows.length) return null;

  return (
    <div>
      {/* The bars carry two meanings on one hue, so the legend is what makes
          them readable; it drops the second swatch when there is nothing to
          compare against. */}
      <div className="mb-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-500">
        <span className="flex items-center gap-1.5">
          <span aria-hidden className="h-2 w-4 rounded-full bg-[#9ec5f4]" />
          Reserved CPU — bar length compares the groups; the biggest fills the row
        </span>
        {haveUsage ? (
          <span className="flex items-center gap-1.5">
            <span aria-hidden className="h-2 w-4 rounded-full bg-[#1c5cab]" />
            In use now — the part of its own reservation each group is actually using
          </span>
        ) : (
          <span>No Prometheus, so what is in use cannot be shown</span>
        )}
      </div>
      <ul className="space-y-3">
        {rows.map((row) => {
          const reservedCores = row.cpuMillis / 1000;
          const share = row.cpuMillis / widest;
          const usedShare = reservedCores > 0 ? Math.min(1, row.usedCores / reservedCores) : 0;
          return (
            <li key={row.key}>
              <div className="flex flex-col gap-0.5 text-sm sm:flex-row sm:items-baseline sm:justify-between sm:gap-3">
                <span className="min-w-0 truncate text-slate-700" title={row.key}>
                  {row.key}
                </span>
                <span className="shrink-0 tabular-nums text-slate-500">
                  {haveUsage && (
                    <>
                      <strong className="font-semibold text-slate-900">
                        {formatUsedCores(row.usedCores)}
                      </strong>{' '}
                      of{' '}
                    </>
                  )}
                  <span className={haveUsage ? '' : 'font-semibold text-slate-900'}>
                    {formatCores(row.cpuMillis)}
                  </span>{' '}
                  cores · {formatBytes(row.memBytes)}
                  {showGpu && row.gpu ? ` · ${row.gpu} GPU` : ''} · {row.count}{' '}
                  {row.count === 1 ? 'sandbox' : 'sandboxes'}
                </span>
              </div>
              <div className="mt-1 h-2.5 w-full rounded-full bg-slate-100">
                <div
                  className="h-full rounded-full bg-[#9ec5f4]"
                  style={{ width: `${Math.max(share * 100, 1)}%` }}
                  title={`${formatCores(row.cpuMillis)} cores reserved (the biggest group reserves ${formatCores(widest)})`}
                >
                  {haveUsage && (
                    <div
                      className="h-full rounded-full bg-[#1c5cab]"
                      style={{ width: `${Math.max(usedShare * 100, row.usedCores > 0 ? 2 : 0)}%` }}
                      title={`${formatUsedCores(row.usedCores)} of ${formatCores(row.cpuMillis)} reserved cores in use (${Math.round(usedShare * 100)}%)`}
                    />
                  )}
                </div>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

/**
 * A share view for a dimension with a long tail — a ring cannot show sixty
 * images, but a ranked list of the largest few, with the rest folded into one
 * row, says exactly how the fleet is spread.
 */
export function ShareList({
  slices,
  total,
  selected,
  onSelect,
}: { slices: Slice[]; total: number } & Selectable) {
  if (!total) return <p className="py-8 text-center text-sm text-slate-500">No sandboxes.</p>;

  return (
    <ul className="space-y-2.5">
      {slices.map((s) => {
        const pickable = !!onSelect && s.key !== OTHER_KEY;
        const body = (
          <>
            <div className="flex items-baseline justify-between gap-2 text-sm">
              <span className="flex min-w-0 items-baseline gap-2">
                <span
                  aria-hidden
                  className="h-2 w-2 shrink-0 rounded-full"
                  style={{ background: s.color, opacity: dimmed(s, selected) }}
                />
                <span
                  className={`min-w-0 truncate ${
                    s.key === selected ? 'font-medium text-slate-900' : 'text-slate-700'
                  }`}
                  title={s.key}
                >
                  {s.key}
                </span>
              </span>
              <span className="shrink-0 tabular-nums text-slate-500">
                <strong className="font-semibold text-slate-900">{s.count}</strong>{' '}
                {Math.round((s.count / total) * 100)}%
              </span>
            </div>
            <div className="mt-1 h-2 w-full rounded-full bg-slate-100">
              <div
                className="h-full rounded-full"
                style={{
                  width: `${Math.max((s.count / total) * 100, 1)}%`,
                  background: s.color,
                  opacity: dimmed(s, selected),
                }}
              />
            </div>
          </>
        );
        return (
          <li key={s.key}>
            {pickable ? (
              <button
                type="button"
                onClick={() => onSelect(s.key)}
                aria-pressed={s.key === selected}
                className="block w-full rounded px-1 -mx-1 text-left hover:bg-slate-50"
              >
                {body}
              </button>
            ) : (
              body
            )}
          </li>
        );
      })}
    </ul>
  );
}

/** Live CPU use is often a fraction of a core, where rounding hides the value. */
function formatUsedCores(cores: number): string {
  if (cores === 0) return '0';
  if (cores < 10) return cores.toFixed(2);
  return Math.round(cores).toString();
}
