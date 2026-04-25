const API_BASE = import.meta.env.VITE_API_URL || 'http://localhost:8080';

async function request(path, options = {}) {
  const url = `${API_BASE}${path}`;
  const config = {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  };

  try {
    const res = await fetch(url, config);
    const data = await res.json();
    return { data, status: res.status, ok: res.ok };
  } catch (err) {
    return { data: null, status: 0, ok: false, error: err.message };
  }
}

export const api = {
  // Health & Stats
  health: () => request('/api/v1/health'),
  stats: (details = false) => request(`/api/v1/stats?details=${details}`),

  // CRUD
  get: (key) => request(`/api/v1/kv/${encodeURIComponent(key)}`),
  put: (key, value) => request(`/api/v1/kv/${encodeURIComponent(key)}`, {
    method: 'PUT',
    body: JSON.stringify({ value }),
  }),
  delete: (key) => request(`/api/v1/kv/${encodeURIComponent(key)}`, {
    method: 'DELETE',
  }),
  exists: (key) => request(`/api/v1/kv/${encodeURIComponent(key)}`, {
    method: 'HEAD',
  }),

  // List
  list: (prefix = '', limit = 50, keysOnly = false) =>
    request(`/api/v1/kv?prefix=${encodeURIComponent(prefix)}&limit=${limit}&keys_only=${keysOnly}`),

  // Batch
  batchPut: (items) => request('/api/v1/kv/batch/put', {
    method: 'POST',
    body: JSON.stringify({ items }),
  }),
  batchGet: (keys) => request('/api/v1/kv/batch/get', {
    method: 'POST',
    body: JSON.stringify({ keys }),
  }),
  batchDelete: (keys) => request('/api/v1/kv/batch/delete', {
    method: 'POST',
    body: JSON.stringify({ keys }),
  }),
};
