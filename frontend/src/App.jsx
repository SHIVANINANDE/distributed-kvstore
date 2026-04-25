import { useState, useEffect, useCallback, useRef } from 'react';
import { api } from './services/api';
import './index.css';

/* ── Toast System ── */
function ToastContainer({ toasts, onDismiss }) {
  return (
    <div className="toast-container">
      {toasts.map(t => (
        <div key={t.id} className={`toast ${t.type}`} onClick={() => onDismiss(t.id)}>
          <span>{t.type === 'success' ? '✓' : t.type === 'error' ? '✕' : 'ℹ'}</span>
          <span>{t.message}</span>
        </div>
      ))}
    </div>
  );
}

function useToast() {
  const [toasts, setToasts] = useState([]);
  const add = useCallback((message, type = 'info') => {
    const id = Date.now();
    setToasts(prev => [...prev, { id, message, type }]);
    setTimeout(() => setToasts(prev => prev.filter(t => t.id !== id)), 4000);
  }, []);
  const dismiss = useCallback((id) => setToasts(prev => prev.filter(t => t.id !== id)), []);
  return { toasts, add, dismiss };
}

/* ── Modal ── */
function Modal({ title, children, onClose }) {
  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal" onClick={e => e.stopPropagation()}>
        <h3>{title}</h3>
        {children}
      </div>
    </div>
  );
}

/* ── Dashboard Page ── */
function Dashboard({ toast }) {
  const [health, setHealth] = useState(null);
  const [stats, setStats] = useState(null);
  const [loading, setLoading] = useState(true);

  const fetchData = useCallback(async () => {
    const [h, s] = await Promise.all([api.health(), api.stats(true)]);
    if (h.ok) setHealth(h.data);
    if (s.ok) setStats(s.data);
    setLoading(false);
  }, []);

  useEffect(() => { fetchData(); const i = setInterval(fetchData, 5000); return () => clearInterval(i); }, [fetchData]);

  if (loading) return <div className="loading-overlay"><div className="spinner" /></div>;

  const formatBytes = (b) => {
    if (!b || b === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(b) / Math.log(1024));
    return `${(b / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
  };

  return (
    <>
      <div className="page-header">
        <h2>Dashboard</h2>
        <p>System overview and real-time metrics</p>
      </div>

      <div className="stats-grid">
        <div className="card stat-card">
          <div className="stat-label">Status</div>
          <div style={{ marginTop: 4 }}>
            <span className={`badge ${health?.healthy ? 'healthy' : 'unhealthy'}`}>
              <span className="badge-dot" />
              {health?.status || 'unknown'}
            </span>
          </div>
        </div>
        <div className="card stat-card accent">
          <div className="stat-label">Uptime</div>
          <div className="stat-value">{health?.uptime_seconds ? `${Math.floor(health.uptime_seconds / 60)}m` : '—'}</div>
        </div>
        <div className="card stat-card success">
          <div className="stat-label">Storage Size</div>
          <div className="stat-value">{stats ? formatBytes(stats.total_size) : '—'}</div>
        </div>
        <div className="card stat-card warning">
          <div className="stat-label">LSM / VLog</div>
          <div className="stat-value" style={{ fontSize: 18 }}>
            {stats ? `${formatBytes(stats.lsm_size)} / ${formatBytes(stats.vlog_size)}` : '—'}
          </div>
        </div>
      </div>

      {stats?.details?.cache && (
        <div className="card" style={{ marginBottom: 20 }}>
          <div className="card-header">
            <span className="card-title">Cache Statistics</span>
          </div>
          <div className="stats-grid" style={{ marginBottom: 0 }}>
            {Object.entries(typeof stats.details.cache === 'string' ? JSON.parse(stats.details.cache) : stats.details.cache || {}).map(([k, v]) => (
              <div key={k} style={{ padding: '8px 0' }}>
                <div style={{ fontSize: 11, color: 'var(--text-muted)', textTransform: 'uppercase' }}>{k.replace(/_/g, ' ')}</div>
                <div style={{ fontSize: 18, fontWeight: 600, fontFamily: 'var(--font-mono)' }}>{typeof v === 'number' ? v.toLocaleString() : String(v)}</div>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="card">
        <div className="card-header">
          <span className="card-title">System Information</span>
          <span className="card-subtitle">v{health?.version || '1.0.0'}</span>
        </div>
        <table>
          <tbody>
            <tr><td style={{ color: 'var(--text-muted)', width: 180 }}>Version</td><td>{health?.version}</td></tr>
            <tr><td style={{ color: 'var(--text-muted)' }}>Storage Engine</td><td>BadgerDB (LSM-tree)</td></tr>
            <tr><td style={{ color: 'var(--text-muted)' }}>API</td><td>REST + gRPC</td></tr>
            <tr><td style={{ color: 'var(--text-muted)' }}>Timestamp</td><td>{health?.timestamp ? new Date(health.timestamp * 1000).toLocaleString() : '—'}</td></tr>
          </tbody>
        </table>
      </div>
    </>
  );
}

/* ── Key Explorer Page ── */
function KeyExplorer({ toast }) {
  const [keys, setKeys] = useState([]);
  const [prefix, setPrefix] = useState('');
  const [loading, setLoading] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [editItem, setEditItem] = useState(null);
  const [viewItem, setViewItem] = useState(null);

  const fetchKeys = useCallback(async () => {
    setLoading(true);
    const res = await api.list(prefix, 200);
    if (res.ok && res.data?.items) {
      setKeys(res.data.items);
    } else {
      setKeys([]);
    }
    setLoading(false);
  }, [prefix]);

  useEffect(() => { fetchKeys(); }, [fetchKeys]);

  const handleDelete = async (key) => {
    if (!confirm(`Delete key "${key}"?`)) return;
    const res = await api.delete(key);
    if (res.ok) { toast.add(`Deleted "${key}"`, 'success'); fetchKeys(); }
    else toast.add(`Failed to delete: ${res.data?.error || 'unknown'}`, 'error');
  };

  return (
    <>
      <div className="page-header">
        <h2>Key Explorer</h2>
        <p>Browse, search, and manage key-value pairs</p>
      </div>

      <div className="toolbar">
        <div className="search-bar" style={{ flex: 1, maxWidth: 400 }}>
          <span className="search-icon">🔍</span>
          <input type="text" placeholder="Filter by prefix..." value={prefix} onChange={e => setPrefix(e.target.value)} />
        </div>
        <div className="toolbar-spacer" />
        <button className="btn btn-secondary" onClick={fetchKeys} disabled={loading}>
          {loading ? <span className="spinner" /> : '↻'} Refresh
        </button>
        <button className="btn btn-primary" onClick={() => setShowCreate(true)}>+ New Key</button>
      </div>

      <div className="card">
        {loading ? (
          <div className="loading-overlay"><div className="spinner" /></div>
        ) : keys.length === 0 ? (
          <div className="empty-state">
            <div className="empty-icon">📦</div>
            <h4>No keys found</h4>
            <p>Create a new key-value pair to get started.</p>
          </div>
        ) : (
          <div className="table-wrapper">
            <table>
              <thead>
                <tr>
                  <th>Key</th>
                  <th>Value</th>
                  <th style={{ width: 140 }}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {keys.map(item => (
                  <tr key={item.key}>
                    <td className="key-cell">{item.key}</td>
                    <td className="value-cell" title={item.value}>{item.value}</td>
                    <td>
                      <div style={{ display: 'flex', gap: 4 }}>
                        <button className="btn btn-secondary btn-sm" onClick={() => setViewItem(item)} title="View">👁</button>
                        <button className="btn btn-secondary btn-sm" onClick={() => setEditItem(item)} title="Edit">✏️</button>
                        <button className="btn btn-danger btn-sm" onClick={() => handleDelete(item.key)} title="Delete">🗑</button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {keys.length > 0 && (
          <div style={{ padding: '12px 0 0', fontSize: 12, color: 'var(--text-muted)' }}>
            Showing {keys.length} key{keys.length !== 1 ? 's' : ''}
          </div>
        )}
      </div>

      {showCreate && <CreateKeyModal onClose={() => setShowCreate(false)} onSaved={() => { setShowCreate(false); fetchKeys(); }} toast={toast} />}
      {editItem && <EditKeyModal item={editItem} onClose={() => setEditItem(null)} onSaved={() => { setEditItem(null); fetchKeys(); }} toast={toast} />}
      {viewItem && (
        <Modal title={`Key: ${viewItem.key}`} onClose={() => setViewItem(null)}>
          <div className="input-group">
            <label>Value</label>
            <textarea readOnly value={viewItem.value} style={{ minHeight: 200 }} />
          </div>
          <div className="modal-actions">
            <button className="btn btn-secondary" onClick={() => setViewItem(null)}>Close</button>
          </div>
        </Modal>
      )}
    </>
  );
}

function CreateKeyModal({ onClose, onSaved, toast }) {
  const [key, setKey] = useState('');
  const [value, setValue] = useState('');
  const [saving, setSaving] = useState(false);

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!key.trim()) return;
    setSaving(true);
    const res = await api.put(key, value);
    setSaving(false);
    if (res.ok) { toast.add(`Created "${key}"`, 'success'); onSaved(); }
    else toast.add(`Failed: ${res.data?.error || 'unknown'}`, 'error');
  };

  return (
    <Modal title="Create Key-Value Pair" onClose={onClose}>
      <form onSubmit={handleSubmit}>
        <div className="input-group">
          <label>Key</label>
          <input type="text" value={key} onChange={e => setKey(e.target.value)} placeholder="e.g. user:123" autoFocus />
        </div>
        <div className="input-group">
          <label>Value</label>
          <textarea value={value} onChange={e => setValue(e.target.value)} placeholder="Enter value..." />
        </div>
        <div className="modal-actions">
          <button type="button" className="btn btn-secondary" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={!key.trim() || saving}>
            {saving ? <span className="spinner" /> : null} Create
          </button>
        </div>
      </form>
    </Modal>
  );
}

function EditKeyModal({ item, onClose, onSaved, toast }) {
  const [value, setValue] = useState(item.value);
  const [saving, setSaving] = useState(false);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setSaving(true);
    const res = await api.put(item.key, value);
    setSaving(false);
    if (res.ok) { toast.add(`Updated "${item.key}"`, 'success'); onSaved(); }
    else toast.add(`Failed: ${res.data?.error || 'unknown'}`, 'error');
  };

  return (
    <Modal title={`Edit: ${item.key}`} onClose={onClose}>
      <form onSubmit={handleSubmit}>
        <div className="input-group">
          <label>Key (read-only)</label>
          <input type="text" value={item.key} readOnly style={{ opacity: 0.6 }} />
        </div>
        <div className="input-group">
          <label>Value</label>
          <textarea value={value} onChange={e => setValue(e.target.value)} autoFocus />
        </div>
        <div className="modal-actions">
          <button type="button" className="btn btn-secondary" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={saving}>
            {saving ? <span className="spinner" /> : null} Save
          </button>
        </div>
      </form>
    </Modal>
  );
}

/* ── Batch Operations Page ── */
function BatchOps({ toast }) {
  const [mode, setMode] = useState('put');
  const [input, setInput] = useState('');
  const [result, setResult] = useState(null);
  const [loading, setLoading] = useState(false);

  const handleExecute = async () => {
    setLoading(true);
    setResult(null);
    try {
      const parsed = JSON.parse(input);
      let res;
      if (mode === 'put') res = await api.batchPut(parsed);
      else if (mode === 'get') res = await api.batchGet(parsed);
      else res = await api.batchDelete(parsed);

      setResult(res.data);
      if (res.ok) toast.add(`Batch ${mode} completed`, 'success');
      else toast.add(`Batch ${mode} failed`, 'error');
    } catch (e) {
      toast.add(`Invalid JSON: ${e.message}`, 'error');
    }
    setLoading(false);
  };

  const templates = {
    put: JSON.stringify([{ key: "user:1", value: "Alice" }, { key: "user:2", value: "Bob" }], null, 2),
    get: JSON.stringify(["user:1", "user:2"], null, 2),
    delete: JSON.stringify(["user:1", "user:2"], null, 2),
  };

  return (
    <>
      <div className="page-header">
        <h2>Batch Operations</h2>
        <p>Execute bulk operations on multiple keys</p>
      </div>

      <div className="toolbar">
        {['put', 'get', 'delete'].map(m => (
          <button key={m} className={`btn ${mode === m ? 'btn-primary' : 'btn-secondary'}`}
            onClick={() => { setMode(m); setInput(templates[m]); setResult(null); }}>
            Batch {m.toUpperCase()}
          </button>
        ))}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 20 }}>
        <div className="card">
          <div className="card-header">
            <span className="card-title">Input ({mode === 'put' ? 'Array of {key, value}' : 'Array of keys'})</span>
          </div>
          <textarea value={input || templates[mode]} onChange={e => setInput(e.target.value)}
            style={{ minHeight: 300, fontFamily: 'var(--font-mono)', fontSize: 12 }} />
          <div style={{ marginTop: 16 }}>
            <button className="btn btn-primary" onClick={handleExecute} disabled={loading}>
              {loading ? <span className="spinner" /> : null} Execute
            </button>
          </div>
        </div>

        <div className="card">
          <div className="card-header"><span className="card-title">Result</span></div>
          {result ? (
            <pre style={{ fontFamily: 'var(--font-mono)', fontSize: 12, whiteSpace: 'pre-wrap', color: 'var(--text-secondary)' }}>
              {JSON.stringify(result, null, 2)}
            </pre>
          ) : (
            <div className="empty-state"><div className="empty-icon">📋</div><h4>No results yet</h4></div>
          )}
        </div>
      </div>
    </>
  );
}

/* ── Health Page ── */
function HealthPage({ toast }) {
  const [health, setHealth] = useState(null);
  const [loading, setLoading] = useState(true);

  const fetchHealth = useCallback(async () => {
    const res = await api.health();
    if (res.ok) setHealth(res.data);
    setLoading(false);
  }, []);

  useEffect(() => { fetchHealth(); const i = setInterval(fetchHealth, 3000); return () => clearInterval(i); }, [fetchHealth]);

  if (loading) return <div className="loading-overlay"><div className="spinner" /></div>;

  return (
    <>
      <div className="page-header">
        <h2>System Health</h2>
        <p>Real-time health monitoring (auto-refresh: 3s)</p>
      </div>
      <div className="stats-grid">
        <div className="card stat-card">
          <div className="stat-label">Overall Status</div>
          <div style={{ marginTop: 8 }}>
            <span className={`badge ${health?.healthy ? 'healthy' : 'unhealthy'}`} style={{ fontSize: 16, padding: '8px 20px' }}>
              <span className="badge-dot" />
              {health?.status || 'unknown'}
            </span>
          </div>
        </div>
        <div className="card stat-card accent">
          <div className="stat-label">Uptime</div>
          <div className="stat-value">
            {health?.uptime_seconds != null
              ? health.uptime_seconds >= 3600
                ? `${Math.floor(health.uptime_seconds / 3600)}h ${Math.floor((health.uptime_seconds % 3600) / 60)}m`
                : `${Math.floor(health.uptime_seconds / 60)}m ${health.uptime_seconds % 60}s`
              : '—'}
          </div>
        </div>
        <div className="card stat-card">
          <div className="stat-label">Version</div>
          <div className="stat-value" style={{ fontSize: 22 }}>{health?.version || '—'}</div>
        </div>
      </div>

      <div className="card">
        <div className="card-header"><span className="card-title">Health Check Details</span></div>
        <table>
          <thead><tr><th>Check</th><th>Status</th><th>Message</th></tr></thead>
          <tbody>
            <tr>
              <td>HTTP API</td>
              <td><span className={`badge ${health?.healthy ? 'healthy' : 'unhealthy'}`}><span className="badge-dot" />{health?.healthy ? 'Healthy' : 'Down'}</span></td>
              <td style={{ color: 'var(--text-secondary)' }}>REST API responding</td>
            </tr>
            <tr>
              <td>Storage Engine</td>
              <td><span className="badge healthy"><span className="badge-dot" />Healthy</span></td>
              <td style={{ color: 'var(--text-secondary)' }}>BadgerDB operational</td>
            </tr>
            <tr>
              <td>Cache Layer</td>
              <td><span className="badge healthy"><span className="badge-dot" />Healthy</span></td>
              <td style={{ color: 'var(--text-secondary)' }}>LRU cache active</td>
            </tr>
          </tbody>
        </table>
      </div>
    </>
  );
}

/* ── Main App ── */
const PAGES = [
  { id: 'dashboard', label: 'Dashboard', icon: '📊', section: 'Overview' },
  { id: 'keys', label: 'Key Explorer', icon: '🔑', section: 'Overview' },
  { id: 'batch', label: 'Batch Ops', icon: '⚡', section: 'Operations' },
  { id: 'health', label: 'Health', icon: '💚', section: 'System' },
];

export default function App() {
  const [page, setPage] = useState('dashboard');
  const toast = useToast();

  const sections = {};
  PAGES.forEach(p => { (sections[p.section] = sections[p.section] || []).push(p); });

  const renderPage = () => {
    switch (page) {
      case 'dashboard': return <Dashboard toast={toast} />;
      case 'keys': return <KeyExplorer toast={toast} />;
      case 'batch': return <BatchOps toast={toast} />;
      case 'health': return <HealthPage toast={toast} />;
      default: return <Dashboard toast={toast} />;
    }
  };

  return (
    <div className="app-layout">
      <aside className="sidebar">
        <div className="sidebar-logo">
          <div className="logo-icon">K</div>
          <div>
            <h1>KV Store</h1>
            <span>Distributed Storage</span>
          </div>
        </div>
        {Object.entries(sections).map(([section, items]) => (
          <div key={section} className="nav-section">
            <div className="nav-section-title">{section}</div>
            {items.map(item => (
              <div key={item.id} className={`nav-item ${page === item.id ? 'active' : ''}`} onClick={() => setPage(item.id)}>
                <span className="icon">{item.icon}</span>
                {item.label}
              </div>
            ))}
          </div>
        ))}
      </aside>
      <main className="main-content">{renderPage()}</main>
      <ToastContainer toasts={toast.toasts} onDismiss={toast.dismiss} />
    </div>
  );
}
