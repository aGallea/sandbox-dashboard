import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter } from 'react-router-dom';
import { App } from './App';
import { basePath } from './api/basePath';
import './index.css';

const qc = new QueryClient();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={qc}>
      {/* Client-side routes are relative to wherever the dashboard is mounted. */}
      <BrowserRouter basename={basePath()}>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);
