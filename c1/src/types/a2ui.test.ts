import { describe, it, expect } from 'vitest';
import { isA2UISpec } from './a2ui';

describe('isA2UISpec', () => {
  it('returns true for valid A2UISpec with items', () => {
    expect(isA2UISpec({ type: 'actions', items: [] })).toBe(true);
  });

  it('returns true with title field', () => {
    expect(isA2UISpec({ type: 'actions', title: 'Choose action', items: [{ id: '1', label: 'OK', style: 'primary' }] })).toBe(true);
  });

  it('returns false for null', () => {
    expect(isA2UISpec(null)).toBe(false);
  });

  it('returns false for undefined', () => {
    expect(isA2UISpec(undefined)).toBe(false);
  });

  it('returns false for a plain string', () => {
    expect(isA2UISpec('actions')).toBe(false);
  });

  it('returns false for a number', () => {
    expect(isA2UISpec(42)).toBe(false);
  });

  it('returns false when type is not "actions"', () => {
    expect(isA2UISpec({ type: 'buttons', items: [] })).toBe(false);
  });

  it('returns false when items is missing', () => {
    expect(isA2UISpec({ type: 'actions' })).toBe(false);
  });

  it('returns false when items is not an array', () => {
    expect(isA2UISpec({ type: 'actions', items: 'not-array' })).toBe(false);
  });

  it('returns false for an empty object', () => {
    expect(isA2UISpec({})).toBe(false);
  });
});
