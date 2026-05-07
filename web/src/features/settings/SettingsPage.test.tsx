import '@testing-library/jest-dom/vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { SETTINGS_STORAGE_KEYS } from './settingsStore';
import { SettingsPage } from './SettingsPage';

describe('SettingsPage', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('saves settings and shows a success message', () => {
    render(<SettingsPage />);

    fireEvent.change(screen.getByLabelText('トランスポンダーID'), {
      target: { value: '0x2a' },
    });
    fireEvent.click(screen.getByLabelText('音声読み上げを有効にする'));
    fireEvent.click(screen.getByRole('button', { name: '保存' }));

    expect(window.localStorage.getItem(SETTINGS_STORAGE_KEYS.transponder)).toBe('42');
    expect(window.localStorage.getItem(SETTINGS_STORAGE_KEYS.speechEnabled)).toBe('true');
    expect(screen.getByRole('status')).toHaveTextContent('保存しました');
    expect(screen.getByLabelText('トランスポンダーID')).toHaveValue('42');
  });

  it('restores saved settings', () => {
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.transponder, '12345');
    window.localStorage.setItem(SETTINGS_STORAGE_KEYS.speechEnabled, 'true');

    render(<SettingsPage />);

    expect(screen.getByLabelText('トランスポンダーID')).toHaveValue('12345');
    expect(screen.getByLabelText('音声読み上げを有効にする')).toBeChecked();
  });

  it('shows a validation error and does not save invalid input', () => {
    render(<SettingsPage />);

    fireEvent.change(screen.getByLabelText('トランスポンダーID'), {
      target: { value: '0x100000000' },
    });
    fireEvent.click(screen.getByRole('button', { name: '保存' }));

    expect(screen.getByRole('alert')).toHaveTextContent('0から0xFFFFFFFF');
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(window.localStorage.getItem(SETTINGS_STORAGE_KEYS.transponder)).toBeNull();
    expect(window.localStorage.getItem(SETTINGS_STORAGE_KEYS.speechEnabled)).toBeNull();
  });

  describe('recent transponders (#110)', () => {
    it('hides the section when no recent transponders exist', () => {
      render(<SettingsPage />);
      expect(screen.queryByText('最近使った')).not.toBeInTheDocument();
    });

    it('shows pre-existing recents from localStorage', () => {
      window.localStorage.setItem(
        SETTINGS_STORAGE_KEYS.recentTransponders,
        JSON.stringify([12345678, 87654321, 99]),
      );
      render(<SettingsPage />);
      expect(screen.getByText('最近使った')).toBeInTheDocument();
      expect(screen.getByLabelText('12345678 を入力欄に入れる')).toBeInTheDocument();
      expect(screen.getByLabelText('87654321 を入力欄に入れる')).toBeInTheDocument();
      expect(screen.getByLabelText('99 を入力欄に入れる')).toBeInTheDocument();
    });

    it('appends a new value to the recents list after a successful save', () => {
      render(<SettingsPage />);
      fireEvent.change(screen.getByLabelText('トランスポンダーID'), {
        target: { value: '777' },
      });
      fireEvent.click(screen.getByRole('button', { name: '保存' }));

      expect(window.localStorage.getItem(SETTINGS_STORAGE_KEYS.recentTransponders)).toBe(
        JSON.stringify([777]),
      );
      expect(screen.getByLabelText('777 を入力欄に入れる')).toBeInTheDocument();
    });

    it('moves an already-saved value to the front instead of duplicating', () => {
      window.localStorage.setItem(
        SETTINGS_STORAGE_KEYS.recentTransponders,
        JSON.stringify([111, 222, 333]),
      );
      render(<SettingsPage />);

      fireEvent.change(screen.getByLabelText('トランスポンダーID'), {
        target: { value: '222' },
      });
      fireEvent.click(screen.getByRole('button', { name: '保存' }));

      expect(window.localStorage.getItem(SETTINGS_STORAGE_KEYS.recentTransponders)).toBe(
        JSON.stringify([222, 111, 333]),
      );
    });

    it('fills the input on chip click without auto-saving', () => {
      window.localStorage.setItem(
        SETTINGS_STORAGE_KEYS.recentTransponders,
        JSON.stringify([5555, 4444]),
      );
      render(<SettingsPage />);

      fireEvent.click(screen.getByLabelText('5555 を入力欄に入れる'));

      expect(screen.getByLabelText('トランスポンダーID')).toHaveValue('5555');
      // Picking a chip is fill-only — current value in storage is unchanged
      // until the user presses 保存.
      expect(window.localStorage.getItem(SETTINGS_STORAGE_KEYS.transponder)).toBeNull();
      expect(screen.queryByRole('status')).not.toBeInTheDocument();
    });

    it('removes a single chip with the ✕ button', () => {
      window.localStorage.setItem(
        SETTINGS_STORAGE_KEYS.recentTransponders,
        JSON.stringify([111, 222, 333]),
      );
      render(<SettingsPage />);

      fireEvent.click(screen.getByLabelText('222 を履歴から削除'));

      expect(window.localStorage.getItem(SETTINGS_STORAGE_KEYS.recentTransponders)).toBe(
        JSON.stringify([111, 333]),
      );
      expect(screen.queryByLabelText('222 を入力欄に入れる')).not.toBeInTheDocument();
      expect(screen.getByLabelText('111 を入力欄に入れる')).toBeInTheDocument();
    });

    it('clears all recents and hides the section', () => {
      window.localStorage.setItem(
        SETTINGS_STORAGE_KEYS.recentTransponders,
        JSON.stringify([111, 222]),
      );
      render(<SettingsPage />);

      fireEvent.click(screen.getByLabelText('最近使ったトランスポンダーをすべて消去'));

      expect(window.localStorage.getItem(SETTINGS_STORAGE_KEYS.recentTransponders)).toBeNull();
      expect(screen.queryByText('最近使った')).not.toBeInTheDocument();
    });
  });
});
