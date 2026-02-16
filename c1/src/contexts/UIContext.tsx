import React, { createContext, useContext, useState } from 'react';

interface UIContextType {
  isChatOpen: boolean;
  toggleChat: () => void;
  isPaletteOpen: boolean;
  setPaletteOpen: (open: boolean) => void;
}

const UIContext = createContext<UIContextType | undefined>(undefined);

export function UIProvider({ children }: { children: React.ReactNode }) {
  const [isChatOpen, setIsChatOpen] = useState(false);
  const [isPaletteOpen, setIsPaletteOpen] = useState(false);

  const toggleChat = () => setIsChatOpen(prev => !prev);

  return (
    <UIContext.Provider value={{ 
      isChatOpen, 
      toggleChat, 
      isPaletteOpen, 
      setPaletteOpen 
    }}>
      {children}
    </UIContext.Provider>
  );
}

export function useUI() {
  const context = useContext(UIContext);
  if (context === undefined) {
    throw new Error('useUI must be used within a UIProvider');
  }
  return context;
}
