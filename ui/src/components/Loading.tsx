/** Spinner + label for any in-flight fetch. Tailwind's animate-spin does the work. */
export function Loading({
  label = 'Loading…',
  className = '',
}: {
  label?: string;
  className?: string;
}) {
  return (
    <div role="status" className={`flex items-center gap-2 text-sm text-slate-500 ${className}`}>
      <Spinner />
      {label}
    </div>
  );
}

export function Spinner({ className = 'h-4 w-4' }: { className?: string }) {
  return (
    <svg className={`${className} animate-spin shrink-0`} viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" opacity="0.25" />
      <path d="M22 12a10 10 0 0 0-10-10" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  );
}
