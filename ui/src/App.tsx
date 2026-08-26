import { lazy, Suspense, useSyncExternalStore } from 'react';
import { Link, NavLink, Route, Routes } from 'react-router-dom';
import { OverviewPage } from './pages/Overview';
import { ResourceListPage } from './pages/ResourceList';
import { RESOURCES } from './resources/config';
import { Loading } from './components/Loading';
import {
  DEFAULT_REFRESH_MS,
  REFRESH_OPTIONS,
  refreshMs,
  setRefreshMs,
  subscribeRefresh,
} from './api/refresh';

const MetricsPage = lazy(() =>
  import('./pages/Metrics').then((m) => ({ default: m.MetricsPage })),
);

export function App() {
  return (
    <div className="min-h-screen">
      {/*
        z-50 clears the list page's own sticky toolbar (z-30) and the detail
        drawer (z-40), so nav stays reachable with either of them on screen.
      */}
      <header className="sticky top-0 z-50 flex flex-wrap items-baseline gap-x-6 gap-y-2 bg-slate-900 px-6 py-3 text-white">
        <Link to="/" className="font-bold">
          sandbox-dashboard
        </Link>
        <nav className="flex gap-4 text-sm">
          <NavLink to="/" end className={navCls}>
            Overview
          </NavLink>
          <NavLink to="/metrics" className={navCls}>
            Metrics
          </NavLink>
          {Object.values(RESOURCES).map((r) => (
            <NavLink key={r.kind} to={`/${r.kind}`} className={navCls}>
              {r.label}
            </NavLink>
          ))}
        </nav>
        <RefreshPicker />
      </header>
      <main>
        <Routes>
          <Route path="/" element={<OverviewPage />} />
          <Route
            path="/metrics"
            element={
              <Suspense fallback={<Loading className="p-6" label="Loading metrics…" />}>
                <MetricsPage />
              </Suspense>
            }
          />
          <Route path="/sandboxes" element={<ResourceListPage kind="sandboxes" />} />
          <Route path="/sandboxes/:namespace/:name" element={<ResourceListPage kind="sandboxes" />} />
          <Route path="/claims" element={<ResourceListPage kind="claims" />} />
          <Route path="/claims/:namespace/:name" element={<ResourceListPage kind="claims" />} />
          <Route path="/templates" element={<ResourceListPage kind="templates" />} />
          <Route path="/templates/:namespace/:name" element={<ResourceListPage kind="templates" />} />
          <Route path="/warmpools" element={<ResourceListPage kind="warmpools" />} />
          <Route path="/warmpools/:namespace/:name" element={<ResourceListPage kind="warmpools" />} />
        </Routes>
      </main>
    </div>
  );
}

function navCls({ isActive }: { isActive: boolean }) {
  return `hover:underline ${isActive ? 'underline underline-offset-4' : ''}`;
}

/**
 * How often the browser re-asks. The server watches Kubernetes, so it always
 * has the current answer — this only controls how often we go and get it.
 *
 * Off earns its place: a list of nine hundred sandboxes reordering under the
 * cursor every five seconds cannot be read, and neither can a chart being
 * compared against another tab.
 */
function RefreshPicker() {
  const ms = useSyncExternalStore(subscribeRefresh, refreshMs, () => DEFAULT_REFRESH_MS);

  return (
    <label className="ml-auto flex items-center gap-2 text-xs text-slate-300">
      <span className="flex items-center gap-1.5">
        <span
          aria-hidden
          className={`h-1.5 w-1.5 rounded-full ${
            ms === 0 ? 'bg-slate-500' : 'animate-pulse bg-emerald-400'
          }`}
        />
        {ms === 0 ? 'Paused' : 'Live'}
      </span>
      <select
        value={ms}
        onChange={(e) => setRefreshMs(Number(e.target.value))}
        aria-label="Refresh interval"
        title="How often the browser re-asks the server for data"
        className="rounded border border-slate-700 bg-slate-800 px-1.5 py-0.5 text-xs text-white"
      >
        {REFRESH_OPTIONS.map((o) => (
          <option key={o.ms} value={o.ms}>
            {o.label}
          </option>
        ))}
      </select>
    </label>
  );
}
