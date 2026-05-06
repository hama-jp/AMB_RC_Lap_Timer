import '@testing-library/jest-dom/vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { AdminLogin } from './AdminLogin';

function renderWithRouter(initialEntry = '/admin/login'): { lastUrl: () => string | null } {
  const currentPath: string | null = initialEntry;
  function PathProbe(): JSX.Element {
    return <div data-testid="probe">{currentPath}</div>;
  }
  // The router updates location internally; we rely on screen-level
  // assertions ("the form went away" / "the destination component rendered")
  // rather than reading the URL out of MemoryRouter directly.
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/admin/login" element={<AdminLogin />} />
        <Route
          path="/admin"
          element={<div data-testid="admin-landing">admin form would go here</div>}
        />
        <Route path="*" element={<PathProbe />} />
      </Routes>
    </MemoryRouter>,
  );
  return { lastUrl: () => currentPath };
}

describe('AdminLogin', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('navigates to /admin on a successful login', async () => {
    const fetch = window.fetch as ReturnType<typeof vi.fn>;
    fetch.mockResolvedValueOnce(new Response(JSON.stringify({ ok: true }), { status: 200 }));
    renderWithRouter();
    fireEvent.change(screen.getByLabelText('passphrase'), { target: { value: 'goodpass' } });
    fireEvent.click(screen.getByRole('button', { name: 'ログイン' }));
    await screen.findByTestId('admin-landing');
    expect(fetch).toHaveBeenCalledWith(
      '/admin/api/login',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('shows the passphrase-mismatch message on 401', async () => {
    const fetch = window.fetch as ReturnType<typeof vi.fn>;
    fetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: 'invalid passphrase' }), { status: 401 }),
    );
    renderWithRouter();
    fireEvent.change(screen.getByLabelText('passphrase'), { target: { value: 'wrong' } });
    fireEvent.click(screen.getByRole('button', { name: 'ログイン' }));
    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('passphrase が違います'),
    );
  });

  it('shows a cooldown message on 429', async () => {
    const fetch = window.fetch as ReturnType<typeof vi.fn>;
    fetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ retry_after_ms: 4000 }), { status: 429 }),
    );
    renderWithRouter();
    fireEvent.change(screen.getByLabelText('passphrase'), { target: { value: 'wrong' } });
    fireEvent.click(screen.getByRole('button', { name: 'ログイン' }));
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('約 4 秒'));
  });

  it('disables the submit button while empty', () => {
    renderWithRouter();
    expect(screen.getByRole('button', { name: 'ログイン' })).toBeDisabled();
    fireEvent.change(screen.getByLabelText('passphrase'), { target: { value: 'x' } });
    expect(screen.getByRole('button', { name: 'ログイン' })).toBeEnabled();
  });
});
