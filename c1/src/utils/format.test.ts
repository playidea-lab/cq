import { describe, it, expect, vi, afterEach } from 'vitest';
import { formatSize, formatDate } from './format';

describe('formatSize', () => {
  it('formats bytes', () => {
    expect(formatSize(0)).toBe('0B');
    expect(formatSize(512)).toBe('512B');
    expect(formatSize(1023)).toBe('1023B');
  });

  it('formats kilobytes', () => {
    expect(formatSize(1024)).toBe('1.0KB');
    expect(formatSize(1536)).toBe('1.5KB');
    expect(formatSize(1024 * 1023)).toBe('1023.0KB');
  });

  it('formats megabytes', () => {
    expect(formatSize(1024 * 1024)).toBe('1.0MB');
    expect(formatSize(57 * 1024 * 1024)).toBe('57.0MB');
  });

  it('formats gigabytes', () => {
    expect(formatSize(1024 * 1024 * 1024)).toBe('1.0GB');
    expect(formatSize(2.5 * 1024 * 1024 * 1024)).toBe('2.5GB');
  });
});

describe('formatDate', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns empty string for null', () => {
    expect(formatDate(null)).toBe('');
  });

  it('returns empty string for 0', () => {
    expect(formatDate(0)).toBe('');
  });

  it('returns time for today', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-02-08T15:00:00'));
    const today = new Date('2026-02-08T10:30:00').getTime();
    expect(formatDate(today)).toMatch(/10:30/);
  });

  it('returns Yesterday for 1 day ago', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-02-08T12:00:00'));
    const yesterday = new Date('2026-02-07T12:00:00').getTime();
    expect(formatDate(yesterday)).toBe('Yesterday');
  });

  it('returns Xd ago for 2-6 days', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-02-08T12:00:00'));
    const threeDaysAgo = new Date('2026-02-05T12:00:00').getTime();
    expect(formatDate(threeDaysAgo)).toBe('3d ago');
  });

  it('returns month/day for 7+ days', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-02-08T12:00:00'));
    const twoWeeksAgo = new Date('2026-01-20T12:00:00').getTime();
    const result = formatDate(twoWeeksAgo);
    // Locale-dependent: "Jan 20" or "1월 20일" etc.
    expect(result).toMatch(/20/);
    expect(result).not.toBe('');
    expect(result).not.toBe('Yesterday');
    expect(result).not.toMatch(/d ago/);
  });
});
