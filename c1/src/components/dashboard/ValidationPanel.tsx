import { useState } from 'react';
import type { ValidationResult } from '../../types';

interface ValidationPanelProps {
  validations: ValidationResult[];
}

function PassIcon() {
  return <span className="validation-panel__icon validation-panel__icon--pass">&#10003;</span>;
}

function FailIcon() {
  return <span className="validation-panel__icon validation-panel__icon--fail">&#10007;</span>;
}

export function ValidationPanel({ validations }: ValidationPanelProps) {
  const [expandedIndex, setExpandedIndex] = useState<number | null>(null);

  if (validations.length === 0) {
    return null;
  }

  const passCount = validations.filter(v => v.passed).length;
  const failCount = validations.length - passCount;

  return (
    <div className="validation-panel">
      <div className="validation-panel__header">
        <h4 className="validation-panel__title">Validations</h4>
        <div className="validation-panel__summary">
          {passCount > 0 && (
            <span className="badge badge--green">{passCount} passed</span>
          )}
          {failCount > 0 && (
            <span className="badge badge--red">{failCount} failed</span>
          )}
        </div>
      </div>
      <div className="validation-panel__list">
        {validations.map((v, index) => (
          <div key={index} className="validation-panel__item">
            <button
              className="validation-panel__item-header"
              onClick={() =>
                setExpandedIndex(expandedIndex === index ? null : index)
              }
            >
              {v.passed ? <PassIcon /> : <FailIcon />}
              <span className="validation-panel__name">{v.name}</span>
              {v.output && (
                <span className="validation-panel__expand">
                  {expandedIndex === index ? '\u25B2' : '\u25BC'}
                </span>
              )}
            </button>
            {expandedIndex === index && v.output && (
              <pre className="validation-panel__output">{v.output}</pre>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
