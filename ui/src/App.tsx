import { lazy, Suspense } from 'react';
import { Link, NavLink, Route, Routes } from 'react-router-dom';
import { OverviewPage } from './pages/Overview';
import { ResourceListPage } from './pages/ResourceList';
import { RESOURCES } from './resources/config';
import { Loading } from './components/Loading';

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
