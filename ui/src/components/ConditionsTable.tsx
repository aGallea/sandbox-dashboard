import type { Condition } from '../api/client';

export function ConditionsTable({ conditions }: { conditions: Condition[] }) {
  if (!conditions.length) {
    return <div className="text-slate-500 text-sm">No conditions reported.</div>;
  }
  return (
    <table className="w-full text-sm">
      <thead className="text-left text-slate-500">
        <tr>
          <th className="py-1 pr-2">Type</th>
          <th className="py-1 pr-2">Status</th>
          <th className="py-1 pr-2">Reason</th>
          <th className="py-1">Since</th>
        </tr>
      </thead>
      <tbody>
        {conditions.map((c) => (
          <tr key={c.type} className="border-t border-slate-100">
            <td className="py-1 pr-2 font-medium">{c.type}</td>
            <td className="py-1 pr-2">
              <span className={statusClass(c.status)}>{c.status}</span>
            </td>
            <td className="py-1 pr-2 text-slate-600">{c.reason || '—'}</td>
            <td className="py-1 text-slate-500">{formatTime(c.lastTransitionTime)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function statusClass(s: Condition['status']) {
  switch (s) {
    case 'True':
      return 'text-emerald-700';
    case 'False':
      return 'text-amber-700';
    default:
      return 'text-slate-500';
  }
}

function formatTime(ts?: string) {
  if (!ts) return '—';
  return new Date(ts).toLocaleString();
}
