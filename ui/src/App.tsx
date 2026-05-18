import { Route, Routes, Link } from 'react-router-dom';
import { OverviewPage } from './pages/Overview';

export function App() {
  return (
    <div className="min-h-screen">
      <header className="bg-slate-900 text-white px-6 py-3 flex items-baseline gap-4">
        <h1 className="font-bold">agent-sandbox-dashboard</h1>
        <nav className="flex gap-3 text-sm">
          <Link to="/" className="hover:underline">Overview</Link>
        </nav>
      </header>
      <main>
        <Routes>
          <Route path="/" element={<OverviewPage />} />
        </Routes>
      </main>
    </div>
  );
}
