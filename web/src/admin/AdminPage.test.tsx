import '@testing-library/jest-dom/vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { AdminConfig } from './api';
import { AdminPage } from './AdminPage';

const SAMPLE_CONFIG: AdminConfig = {
  listen: ':8080',
  upstream: {
    host: '192.168.1.21',
    port: 5403,
    reconnect: { initial_ms: 1000, max_ms: 30000, jitter_ratio: 0.2 },
  },
  logging: { dir: './logs', max_size_mb: 5, max_backups: 5 },
  records: { dir: './records' },
  replay: { speed: 'realtime' },
  server: { max_clients: 100, client_buffer_len: 64 },
};

function renderApp(initialEntry = '/admin'): void {
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/admin" element={<AdminPage />} />
        <Route
          path="/admin/login"
          element={<div data-testid="login-page">login form would go here</div>}
        />
      </Routes>
    </MemoryRouter>,
  );
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

describe('AdminPage', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('redirects to /admin/login when GET returns 401', async () => {
    const fetch = window.fetch as ReturnType<typeof vi.fn>;
    fetch.mockResolvedValueOnce(jsonResponse({ error: 'unauthenticated' }, 401));
    renderApp();
    await screen.findByTestId('login-page');
  });

  it('renders the form populated from GET /admin/api/config', async () => {
    const fetch = window.fetch as ReturnType<typeof vi.fn>;
    fetch.mockResolvedValueOnce(jsonResponse(SAMPLE_CONFIG));
    renderApp();
    const hostInput = await screen.findByLabelText('upstream.host');
    expect(hostInput).toHaveValue('192.168.1.21');
    expect(screen.getByLabelText('upstream.port')).toHaveValue(5403);
  });

  it('saves changes and shows the changed-fields toast', async () => {
    const fetch = window.fetch as ReturnType<typeof vi.fn>;
    fetch.mockResolvedValueOnce(jsonResponse(SAMPLE_CONFIG));
    renderApp();
    const hostInput = await screen.findByLabelText('upstream.host');
    fireEvent.change(hostInput, { target: { value: '10.99.0.1' } });

    const updated = {
      ...SAMPLE_CONFIG,
      upstream: { ...SAMPLE_CONFIG.upstream, host: '10.99.0.1' },
    };
    fetch.mockResolvedValueOnce(
      jsonResponse({
        applied: ['upstream.host'],
        requires_restart: [],
        config: updated,
        changed_fields: ['upstream.host'],
      }),
    );
    fireEvent.click(screen.getByRole('button', { name: '保存' }));
    await screen.findByText('保存しました(1 項目変更)');
    expect(screen.queryByText(/再起動後に有効になります/)).not.toBeInTheDocument();
    expect(fetch).toHaveBeenLastCalledWith(
      '/admin/api/config',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('shows the requires_restart banner when listen changes', async () => {
    const fetch = window.fetch as ReturnType<typeof vi.fn>;
    fetch.mockResolvedValueOnce(jsonResponse(SAMPLE_CONFIG));
    renderApp();
    const listenInput = await screen.findByLabelText('listen');
    fireEvent.change(listenInput, { target: { value: ':9090' } });
    fetch.mockResolvedValueOnce(
      jsonResponse({
        applied: [],
        requires_restart: ['listen'],
        config: { ...SAMPLE_CONFIG, listen: ':9090' },
        changed_fields: ['listen'],
      }),
    );
    fireEvent.click(screen.getByRole('button', { name: '保存' }));
    const status = await screen.findByRole('status');
    expect(status).toHaveTextContent(/再起動後に有効になります/);
    expect(status).toHaveTextContent(/listen/);
  });

  it('highlights per-field errors on 400', async () => {
    const fetch = window.fetch as ReturnType<typeof vi.fn>;
    fetch.mockResolvedValueOnce(jsonResponse(SAMPLE_CONFIG));
    renderApp();
    await screen.findByLabelText('upstream.host');
    fetch.mockResolvedValueOnce(
      jsonResponse(
        {
          errors: [{ path: 'upstream.port', message: 'must be between 1 and 65535' }],
        },
        400,
      ),
    );
    fireEvent.click(screen.getByRole('button', { name: '保存' }));
    await screen.findByText('must be between 1 and 65535');
    // Editing the bad field clears its error.
    fireEvent.change(screen.getByLabelText('upstream.port'), { target: { value: '5404' } });
    expect(screen.queryByText('must be between 1 and 65535')).not.toBeInTheDocument();
  });

  it('redirects to login when POST returns 401', async () => {
    const fetch = window.fetch as ReturnType<typeof vi.fn>;
    fetch.mockResolvedValueOnce(jsonResponse(SAMPLE_CONFIG));
    renderApp();
    await screen.findByLabelText('upstream.host');
    fetch.mockResolvedValueOnce(jsonResponse({ error: 'unauthenticated' }, 401));
    fireEvent.click(screen.getByRole('button', { name: '保存' }));
    await screen.findByTestId('login-page');
  });

  it('「初期値に戻す」 resets the form buffer to the bundled defaults', async () => {
    const fetch = window.fetch as ReturnType<typeof vi.fn>;
    fetch.mockResolvedValueOnce(
      jsonResponse({
        ...SAMPLE_CONFIG,
        upstream: { ...SAMPLE_CONFIG.upstream, host: 'edited.example' },
      }),
    );
    renderApp();
    const hostInput = await screen.findByLabelText('upstream.host');
    expect(hostInput).toHaveValue('edited.example');
    fireEvent.click(screen.getByRole('button', { name: '初期値に戻す' }));
    expect(screen.getByLabelText('upstream.host')).toHaveValue('192.168.1.21');
    // Reset must NOT POST anything by itself.
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it('logout button posts to /admin/api/logout and redirects to login', async () => {
    const fetch = window.fetch as ReturnType<typeof vi.fn>;
    fetch.mockResolvedValueOnce(jsonResponse(SAMPLE_CONFIG));
    renderApp();
    await screen.findByLabelText('upstream.host');
    fetch.mockResolvedValueOnce(jsonResponse({ ok: true }));
    fireEvent.click(screen.getByRole('button', { name: 'ログアウト' }));
    await waitFor(() =>
      expect(fetch).toHaveBeenCalledWith(
        '/admin/api/logout',
        expect.objectContaining({ method: 'POST' }),
      ),
    );
    await screen.findByTestId('login-page');
  });
});
