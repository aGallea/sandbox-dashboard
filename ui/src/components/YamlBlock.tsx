import { useState } from 'react';
import yaml from 'js-yaml';

export function YamlBlock({ value, defaultOpen = false }: { value: unknown; defaultOpen?: boolean }) {
  const [open, setOpen] = useState(defaultOpen);
  const text = (() => {
    try {
      return yaml.dump(value, { sortKeys: true, lineWidth: 120 });
    } catch {
      return JSON.stringify(value, null, 2);
    }
  })();
  return (
    <details
      open={open}
      onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)}
      className="border border-slate-200 rounded"
    >
      <summary className="cursor-pointer px-3 py-2 text-sm font-medium select-none">
        Spec (YAML)
      </summary>
      <pre className="text-xs bg-slate-50 p-3 overflow-x-auto whitespace-pre">
        {text}
      </pre>
    </details>
  );
}
