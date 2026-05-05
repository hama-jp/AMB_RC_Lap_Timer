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
});
