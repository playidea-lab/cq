export type A2UIActionStyle = 'primary' | 'secondary' | 'danger';

export interface A2UIAction {
  id: string;
  label: string;
  style: A2UIActionStyle;
}

export interface A2UISpec {
  type: 'actions';
  title?: string;
  items: A2UIAction[];
}

export function isA2UISpec(value: unknown): value is A2UISpec {
  return (
    typeof value === 'object' &&
    value !== null &&
    (value as A2UISpec).type === 'actions' &&
    Array.isArray((value as A2UISpec).items)
  );
}
