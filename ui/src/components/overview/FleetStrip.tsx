import { useState } from 'react';
import { Link } from 'react-router-dom';
import type { ResourceSummary } from '../../api/client';
import {
  STATE_COLOR,
  STATE_LABEL,
  VERDICT_LABEL,
  diagnose,
  formatAge,
  formatBytes,
  formatCores,
  stateOf,
} from '../../overview/aggregate';
import { shortId, taskLabel } from '../../list/rows';

/**
 * The whole fleet as one row of cells, oldest first — one cell per sandbox,
 * coloured by state.
 *
 * A fleet of interchangeable short-lived sandboxes has no shape a donut can
 * show: what an operator wants is every sandbox at once, so a wall of green
 * with two red cells locates the two problems in a glance, and the age order
 * puts whatever has outlived its usefulness at the left edge. Cells shrink as
 * the fleet grows, so the strip holds five sandboxes or two thousand.
 */
export function FleetStrip({ items }: { items: ResourceSummary[] }) {
  const oldestFirst = [...items].sort((a, b) => b.ageSeconds - a.ageSeconds);
  const [hover, setHover] = useState<{ it: ResourceSummary; x: number; y: number } | null>(null);

  return (
    <div>
      <div className="flex flex-wrap gap-[3px]">
        {oldestFirst.map((it) => {
          const state = stateOf(it);
          const { verdict, why } = diagnose(it);
          return (
            <Link
              key={`${it.namespace}/${it.name}`}
              to={`/sandboxes/${it.namespace}/${it.name}`}
              // A native title would do this, but it waits a second to appear
              // and truncates — no use for reading a wall of amber cells.
              onMouseEnter={(e) => {
                const box = e.currentTarget.getBoundingClientRect();
                setHover({ it, x: box.left + box.width / 2, y: box.top });
              }}
              onMouseLeave={() => setHover(null)}
              onFocus={(e) => {
                const box = e.currentTarget.getBoundingClientRect();
                setHover({ it, x: box.left + box.width / 2, y: box.top });
              }}
              onBlur={() => setHover(null)}
              className="h-4 w-4 rounded-[3px] transition-transform hover:scale-125 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-slate-900"
              style={{ background: STATE_COLOR[state] }}
            >
              <span className="sr-only">
                {taskLabel(it).label}, {VERDICT_LABEL[verdict]}, {why}, up{' '}
                {formatAge(it.ageSeconds)}
              </span>
            </Link>
          );
        })}
      </div>
      <p className="mt-3 text-xs text-slate-400">
        Oldest first. Each cell is one sandbox — hover for its state, select one to open it.
      </p>
      {hover && <HoverCard {...hover} />}
    </div>
  );
}

const VERDICT_TONE: Record<string, string> = {
  ready: 'text-emerald-700',
  starting: 'text-slate-600',
  stuck: 'text-amber-700',
  failing: 'text-red-700',
};

/**
 * Fixed rather than absolute, so the card escapes the panel's overflow and does
 * not need the strip to become a positioning context. Clamped to the viewport
 * because cells at the right edge would otherwise push it off screen.
 */
function HoverCard({ it, x, y }: { it: ResourceSummary; x: number; y: number }) {
  const { verdict, why } = diagnose(it);
  const { label, token } = taskLabel(it);
  const pod = it.pod;
  const WIDTH = 320;
  const left = Math.min(Math.max(x - WIDTH / 2, 8), window.innerWidth - WIDTH - 8);

  return (
    <div
      role="tooltip"
      style={{ left, top: y, width: WIDTH, transform: 'translateY(-100%)' }}
      className="pointer-events-none fixed z-50 -mt-2 rounded-lg border border-slate-200 bg-white p-3 text-xs shadow-lg"
    >
      <div className="flex items-baseline justify-between gap-2">
        <span className="min-w-0 truncate font-medium text-slate-900">{label}</span>
        <span className={`shrink-0 font-semibold ${VERDICT_TONE[verdict]}`}>
          {VERDICT_LABEL[verdict]}
        </span>
      </div>
      <p className="mt-0.5 text-slate-500">{why}</p>

      <dl className="mt-2 grid grid-cols-[5.5rem_1fr] gap-x-2 gap-y-1 border-t border-slate-100 pt-2 text-slate-600">
        <Row label="Up for">{formatAge(it.ageSeconds)}</Row>
        {/*
          How long in *this* state is the number that separates late from slow —
          unless the verdict line above already said it, which it does whenever
          the state is the reason.
        */}
        {it.osb && !why.includes(it.osb.state) && (
          <Row label={`${it.osb.state} for`}>{formatAge(it.osb.stateAgeSeconds)}</Row>
        )}
        {pod && pod.phase !== 'Running' && <Row label="Pod phase">{pod.phase}</Row>}
        {!pod && <Row label="Pod">none yet</Row>}
        {(pod?.restarts ?? 0) > 0 && (
          <Row label="Restarts">
            <span className="text-red-700">{pod?.restarts}</span>
          </Row>
        )}
        {pod?.image && (
          <Row label="Image">
            <span className="break-all font-mono text-[11px]">{pod.image}</span>
          </Row>
        )}
        {pod && (pod.cpuMillis > 0 || pod.memBytes > 0) && (
          <Row label="Reserved">
            {formatCores(pod.cpuMillis)} cores · {formatBytes(pod.memBytes)}
            {pod.gpu > 0 && ` · ${pod.gpu} GPU`}
          </Row>
        )}
        {pod?.node && (
          <Row label="Node">
            <span className="break-all font-mono text-[11px]">{pod.node}</span>
          </Row>
        )}
        {it.owner && (
          <Row label="Owner">
            {it.owner}
            {it.team && <span className="text-slate-400"> · {it.team}</span>}
          </Row>
        )}
        {it.osb?.message && <Row label="OpenSandbox">{it.osb.message}</Row>}
        {it.osb?.diverged && (
          <Row label="Divergence">
            <span className="text-red-700">
              OpenSandbox says {it.osb.state}, Kubernetes says {STATE_LABEL[stateOf(it)]}
            </span>
          </Row>
        )}
        <Row label="Id">
          <span className="font-mono text-[11px]">{shortId(it.name) || it.name}</span>
          {token && <span className="text-slate-400"> · {token}</span>}
        </Row>
      </dl>
    </div>
  );
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <>
      <dt className="text-slate-400">{label}</dt>
      <dd className="min-w-0">{children}</dd>
    </>
  );
}
