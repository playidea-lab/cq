import { useState } from 'react';
import type { DocType } from '../../types';

interface CreateDocumentDialogProps {
  docType: DocType;
  onConfirm: (name: string, content: string) => Promise<void>;
  onCancel: () => void;
}

const TEMPLATES: Record<DocType, string> = {
  persona: `# Persona Name

## Role
Describe the persona's role.

## Instructions
- Key behavior 1
- Key behavior 2
`,
  skill: `# Skill Name

## Description
What this skill does.

## Steps
1. Step one
2. Step two
`,
  spec: `title: Untitled Spec
status: draft
---
`,
  config: `# Configuration
`,
};

export function CreateDocumentDialog({ docType, onConfirm, onCancel }: CreateDocumentDialogProps) {
  const [name, setName] = useState('');
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleCreate = async () => {
    const trimmed = name.trim();
    if (!trimmed) return;

    setCreating(true);
    setError(null);
    try {
      await onConfirm(trimmed, TEMPLATES[docType]);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setCreating(false);
      return;
    }
    setCreating(false);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && name.trim()) {
      handleCreate();
    } else if (e.key === 'Escape') {
      onCancel();
    }
  };

  return (
    <div className="dialog-overlay" onClick={onCancel}>
      <div className="dialog" onClick={e => e.stopPropagation()}>
        <h3 className="dialog__title">New {docType}</h3>
        <input
          className="dialog__input"
          type="text"
          placeholder={`Enter ${docType} name...`}
          value={name}
          onChange={e => setName(e.target.value)}
          onKeyDown={handleKeyDown}
          autoFocus
        />
        {error && <p className="dialog__error">{error}</p>}
        <div className="dialog__actions">
          <button
            className="btn btn--secondary"
            onClick={onCancel}
            disabled={creating}
          >
            Cancel
          </button>
          <button
            className="btn btn--primary"
            onClick={handleCreate}
            disabled={!name.trim() || creating}
          >
            {creating ? 'Creating...' : 'Create'}
          </button>
        </div>
      </div>
    </div>
  );
}
