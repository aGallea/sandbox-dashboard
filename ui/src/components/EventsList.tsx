import type { EventEntry } from '../api/client';

export function EventsList({ events }: { events: EventEntry[] }) {
  if (!events.length) {
    return <div className="text-slate-500 text-sm">No recent events.</div>;
  }
  return (
    <ul className="text-sm space-y-1">
      {events.map((ev, i) => (
        <li key={i} className="flex items-baseline gap-2 border-t border-slate-100 pt-1">
          <span className="text-xs text-slate-500 tabular-nums w-20 shrink-0">
            {new Date(ev.time).toLocaleTimeString()}
          </span>
          <span
            className={`text-xs px-1 rounded ${
              ev.type === 'Warning' ? 'bg-amber-100 text-amber-800' : 'bg-slate-100 text-slate-700'
            }`}
          >
            {ev.reason}
          </span>
          <span className="text-slate-700">{ev.message}</span>
        </li>
      ))}
    </ul>
  );
}
