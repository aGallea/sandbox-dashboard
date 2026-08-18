import { useState } from 'react';

export type SortDir = 'asc' | 'desc';

export interface FilterProps {
  options: string[];
  selected: Set<string>;
  onToggle: (col: string, value: string) => void;
  onClear: (col: string) => void;
}

export function Pager({
  page,
  pageCount,
  pageSize,
  total,
  sizes,
  onPage,
  onPageSize,
}: {
  page: number;
  pageCount: number;
  pageSize: number;
  total: number;
  sizes: number[];
  onPage: (to: number) => void;
  onPageSize: (size: number) => void;
}) {
  const from = (page - 1) * pageSize + 1;
  const to = Math.min(page * pageSize, total);
  const step = 'rounded border border-slate-300 px-2 leading-6 disabled:opacity-40 disabled:cursor-default hover:bg-slate-100';

  return (
    <div className="flex items-center gap-2 text-sm text-slate-600">
      <select
        className="border border-slate-300 rounded px-2 py-1 text-sm"
        value={pageSize}
        onChange={(e) => onPageSize(Number(e.target.value))}
        title="Rows per page"
      >
        {sizes.map((n) => (
          <option key={n} value={n}>
            {n} / page
          </option>
        ))}
      </select>
      <span className="tabular-nums">
        {from}–{to} of {total}
      </span>
      <button type="button" className={step} onClick={() => onPage(page - 1)} disabled={page <= 1}>
        ‹
      </button>
      <span className="tabular-nums">
        {page} / {pageCount}
      </span>
      <button
        type="button"
        className={step}
        onClick={() => onPage(page + 1)}
        disabled={page >= pageCount}
      >
        ›
      </button>
    </div>
  );
}

export function ColumnHeader({
  label,
  col,
  pad = 'px-3',
  sort,
  filter,
}: {
  label: string;
  col: string;
  pad?: string;
  sort: { key: string; dir: SortDir; onSort: (key: string) => void };
  filter?: FilterProps;
}) {
  const active = sort.key === col;
  return (
    <th
      className={`${pad} py-2`}
      aria-sort={active ? (sort.dir === 'asc' ? 'ascending' : 'descending') : 'none'}
    >
      <span className="flex items-center gap-1">
        <button
          type="button"
          className="inline-flex items-center gap-1 select-none hover:text-slate-900"
          onClick={() => sort.onSort(col)}
        >
          {label}
          <span className={active ? '' : 'invisible'} aria-hidden>
            {sort.dir === 'asc' ? '▲' : '▼'}
          </span>
        </button>
        {filter && <FilterMenu col={col} label={label} {...filter} />}
      </span>
    </th>
  );
}

// <details> is the whole popover mechanism: it opens and closes itself, stays
// open while several boxes are ticked, and the shared name makes the group
// exclusive so opening one column's filter closes the previous one.
function FilterMenu({
  col,
  label,
  options,
  selected,
  onToggle,
  onClear,
}: FilterProps & { col: string; label: string }) {
  const [query, setQuery] = useState('');
  const shown = query
    ? options.filter((o) => o.toLowerCase().includes(query.toLowerCase()))
    : options;

  return (
    <details name="column-filter" className="relative font-normal">
      <summary
        className={`cursor-pointer select-none list-none rounded px-1 [&::-webkit-details-marker]:hidden ${
          selected.size ? 'bg-slate-200 text-slate-700' : 'text-slate-400 hover:text-slate-700'
        }`}
        title={`Filter by ${label}`}
      >
        <span className="inline-flex items-center gap-0.5">
          <svg viewBox="0 0 12 12" className="h-3 w-3" aria-hidden>
            <path d="M1 2h10L7 6.5V11L5 9.5V6.5z" fill="currentColor" />
          </svg>
          {selected.size > 0 && <span className="text-xs tabular-nums">{selected.size}</span>}
        </span>
      </summary>
      <div className="absolute left-0 z-20 mt-1 w-64 rounded border border-slate-300 bg-white p-2 text-slate-700 shadow-lg">
        {options.length > 8 && (
          <input
            className="mb-2 w-full rounded border border-slate-300 px-2 py-1 text-sm"
            placeholder={`Find ${label.toLowerCase()}…`}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        )}
        <div className="max-h-56 overflow-y-auto">
          {shown.length === 0 && (
            <div className="px-1 py-2 text-slate-400">
              {options.length === 0 ? 'no values on these rows' : 'no match'}
            </div>
          )}
          {shown.map((value) => (
            <label
              key={value}
              className="flex cursor-pointer items-center gap-2 rounded px-1 py-0.5 hover:bg-slate-50"
            >
              <input
                type="checkbox"
                checked={selected.has(value)}
                onChange={() => onToggle(col, value)}
              />
              <span className="truncate" title={value}>
                {value}
              </span>
            </label>
          ))}
        </div>
        {selected.size > 0 && (
          <button
            type="button"
            className="mt-2 text-xs text-slate-500 underline hover:text-slate-800"
            onClick={() => onClear(col)}
          >
            clear
          </button>
        )}
      </div>
    </details>
  );
}
