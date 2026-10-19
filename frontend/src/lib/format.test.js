import { describe, expect, it } from 'vitest';
import { formatDateTime, formatShortDate, lastNDays } from './format.js';
import { niceTicks } from '../components/admin/UsageChart.jsx';

describe('format', () => {
  it('memakai jam lokal server dari string ISO, bukan zona browser', () => {
    expect(formatDateTime('2026-10-19T23:45:10.5+08:00')).toBe('19 Okt 2026, 23.45');
    expect(formatDateTime(null)).toBe('—');
  });

  it('lastNDays inklusif hari ini', () => {
    expect(lastNDays(7, new Date(2026, 9, 19))).toEqual({ from: '2026-10-13', to: '2026-10-19' });
    expect(lastNDays(30, new Date(2026, 2, 1))).toEqual({ from: '2026-01-31', to: '2026-03-01' });
  });

  it('formatShortDate', () => {
    expect(formatShortDate('2026-01-05')).toBe('5 Jan');
  });
});

describe('niceTicks', () => {
  it('membulatkan ke angka bersih', () => {
    expect(niceTicks(87)).toEqual({ max: 100, ticks: [0, 25, 50, 75, 100] });
    expect(niceTicks(3)).toEqual({ max: 3, ticks: [0, 1, 2, 3] });
    expect(niceTicks(0).ticks).toEqual([0, 1, 2, 3, 4]);
    expect(niceTicks(123456).max).toBe(150000);
  });
});
