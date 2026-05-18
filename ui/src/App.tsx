import { Link, NavLink, Route, Routes } from 'react-router-dom';
import { MetricsPage } from './pages/Metrics';
import { OverviewPage } from './pages/Overview';
import { ResourceListPage } from './pages/ResourceList';
import { RESOURCES } from './resources/config';

export function App() {
  return (
    <div className="min-h-screen">
      <header className="bg-slate-900 text-white px-6 py-3 flex items-baseline gap-6">
        <Link to="/" className="font-bold">
          agent-sandbox-dashboard
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
          <Route path="/metrics" element={<MetricsPage />} />
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
