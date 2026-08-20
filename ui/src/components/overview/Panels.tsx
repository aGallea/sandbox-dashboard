import { Link } from 'react-router-dom';
import type { Alert } from '../../overview/aggregate';

/**
 * The overview's chrome: a hairline-ruled panel with a small uppercase eyebrow.
 * Hairlines rather than shadows — this page is an instrument panel, and a
 * shadow under every card turns a reading into a UI element.
 */
export function Panel({
  title,
  hint,
  right,
  className = '',
  children,
}: {
  title: string;
  hint?: string;
  right?: React.ReactNode;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <section className={`rounded-xl border border-slate-200 bg-white ${className}`}>
      <header className="flex items-baseline justify-between gap-4 border-b border-slate-100 px-4 py-2.5">
        <div>
          <h2 className="text-[11px] font-semibold uppercase tracking-[0.08em] text-slate-500">
            {title}
          </h2>
          {hint && <p className="mt-0.5 text-xs text-slate-400">{hint}</p>}
        </div>
        {right}
      </header>
      <div className="p-4">{children}</div>
    </section>
  );
}

/** One headline count, with its own breakdown underneath. */
export function StatCard({
  label,
  value,
  to,
  parts,
}: {
  label: string;
  value: number;
  to?: string;
  parts?: { label: string; value: number; color?: string }[];
}) {
  const body = (
    <>
      <div className="flex items-baseline justify-between gap-2">
        <span className="min-w-0 text-[11px] font-semibold uppercase tracking-[0.08em] text-slate-500">
          {label}
        </span>
        <span className="text-3xl leading-none text-slate-900">{value}</span>
      </div>
      <dl className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-500">
        {parts?.map((p) => (
          <div key={p.label} className="flex items-center gap-1.5">
            {p.color && (
              <span
                aria-hidden
                className="h-1.5 w-1.5 shrink-0 rounded-full"
                style={{ background: p.color }}
              />
            )}
            <dt>{p.label}</dt>
            <dd className="tabular-nums text-slate-700">{p.value}</dd>
          </div>
        ))}
      </dl>
    </>
  );

  const shell =
    'rounded-xl border border-slate-200 bg-white p-4 transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-900';
  return to ? (
    <Link to={to} className={`${shell} hover:border-slate-300`}>
      {body}
    </Link>
  ) : (
    <div className={shell}>{body}</div>
  );
}

/**
 * What needs attention, as chips. Each carries an icon and a word as well as a
 * colour, so the state never rests on hue alone, and links to the filtered list
 * wherever the list can reproduce the selection.
 */
export function TriageChips({ alerts }: { alerts: Alert[] }) {
  if (!alerts.length) {
    return (
      <p className="flex items-center gap-2 text-sm text-slate-600">
        <span aria-hidden className="text-[#0ca30c]">
          ✓
        </span>
        Nothing to act on — every sandbox is ready and none has been running over a day.
      </p>
    );
  }

  return (
    <ul className="flex flex-wrap gap-2">
      {alerts.map((a) => {
        const tone =
          a.tone === 'bad'
            ? 'border-red-200 bg-red-50 text-red-900'
            : 'border-amber-200 bg-amber-50 text-amber-900';
        const inner = (
          <>
            <span aria-hidden>{a.tone === 'bad' ? '✕' : '!'}</span>
            <strong className="tabular-nums font-semibold">{a.count}</strong>
            <span className="font-normal">{a.label}</span>
          </>
        );
        return (
          <li key={a.label}>
            {a.to ? (
              <Link
                to={a.to}
                className={`inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-sm hover:brightness-[0.97] ${tone}`}
              >
                {inner}
              </Link>
            ) : (
              <span
                className={`inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-sm ${tone}`}
              >
                {inner}
              </span>
            )}
          </li>
        );
      })}
    </ul>
  );
}

/** A quiet stand-in for a panel whose data source is not configured. */
export function PanelNote({ children }: { children: React.ReactNode }) {
  return <p className="py-6 text-center text-sm text-slate-500">{children}</p>;
}
