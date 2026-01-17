import { useState, useEffect } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { api } from './api/client';
import { Login } from './pages/Login';
import { Dashboard } from './pages/Dashboard';
import { ProbeDetail } from './pages/ProbeDetail';
import type { ProbeConfig } from './api/types';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

function App() {
  const [authenticated, setAuthenticated] = useState(false);
  const [selectedConfig, setSelectedConfig] = useState<ProbeConfig | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const token = api.getToken();
    if (token) {
      api.getStatus()
        .then(() => setAuthenticated(true))
        .catch(() => api.clearToken())
        .finally(() => setLoading(false));
    } else {
      setLoading(false);
    }
  }, []);

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-100">
        <div className="text-gray-500">Loading...</div>
      </div>
    );
  }

  if (!authenticated) {
    return <Login onLogin={() => setAuthenticated(true)} />;
  }

  return (
    <QueryClientProvider client={queryClient}>
      <div className="min-h-screen bg-gray-100">
        {selectedConfig ? (
          <ProbeDetail
            config={selectedConfig}
            onBack={() => setSelectedConfig(null)}
          />
        ) : (
          <Dashboard onProbeClick={setSelectedConfig} />
        )}
      </div>
    </QueryClientProvider>
  );
}

export default App;
