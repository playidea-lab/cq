import { useEffect, useRef } from 'react';
import { Terminal as XTerm } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import { listen } from '@tauri-apps/api/event';
import 'xterm/css/xterm.css';

interface TerminalProps {
  onClear?: () => void;
}

export function Terminal({ onClear }: TerminalProps) {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<XTerm | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);

  useEffect(() => {
    if (!terminalRef.current) return;

    // Initialize xterm.js
    const term = new XTerm({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'JetBrains Mono, Menlo, Monaco, Courier New, monospace',
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
      },
      convertEol: true,
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(terminalRef.current);
    fitAddon.fit();

    xtermRef.current = term;
    fitAddonRef.current = fitAddon;

    term.writeln('\x1b[1;32m[Hands]\x1b[0m AI Execution Host Ready.');

    // Listen for shell output events from Tauri backend
    const unlisten = listen<{ message: string }>('hands-output', (event) => {
      term.write(event.payload.message);
    });

    // Handle window resize
    const handleResize = () => fitAddon.fit();
    window.addEventListener('resize', handleResize);

    return () => {
      term.dispose();
      unlisten.then((f) => f());
      window.removeEventListener('resize', handleResize);
    };
  }, []);

  return (
    <div className="flex flex-col h-full bg-[#1e1e1e] border-t border-[#333]">
      <div className="flex items-center justify-between px-4 py-1 bg-[#252526] border-b border-[#333]">
        <span className="text-xs font-medium text-gray-400 uppercase tracking-wider">AI Terminal</span>
        <button 
          onClick={() => {
            xtermRef.current?.clear();
            onClear?.();
          }}
          className="text-xs text-gray-500 hover:text-gray-300 transition-colors"
        >
          Clear
        </button>
      </div>
      <div ref={terminalRef} className="flex-1 w-full overflow-hidden p-2" />
    </div>
  );
}
