import { Link } from 'react-router-dom';
import type { ResourceSummary } from '../../api/client';
import { STATE_COLOR, STATE_LABEL, formatAge, stateOf } from '../../overview/aggregate';

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

  return (
    <div>
      <div className="flex flex-wrap gap-[3px]">
        {oldestFirst.map((it) => {
          const state = stateOf(it);
          return (
            <Link
              key={`${it.namespace}/${it.name}`}
              to={`/sandboxes/${it.namespace}/${it.name}`}
              title={`${it.name} · ${STATE_LABEL[state]} · up ${formatAge(it.ageSeconds)}${
                it.pod?.node ? ` · ${it.pod.node}` : ''
              }`}
              className="h-4 w-4 rounded-[3px] transition-transform hover:scale-125 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-slate-900"
              style={{ background: STATE_COLOR[state] }}
            >
              <span className="sr-only">
                {it.name}, {STATE_LABEL[state]}, up {formatAge(it.ageSeconds)}
              </span>
            </Link>
          );
        })}
      </div>
      <p className="mt-3 text-xs text-slate-400">
        Oldest first. Each cell is one sandbox — select one to open it.
      </p>
    </div>
  );
}
