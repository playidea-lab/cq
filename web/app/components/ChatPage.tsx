'use client';

import { useState, useCallback } from 'react';
import StreamingChat from './StreamingChat';
import ConversationSidebar, { useConversationManager } from './ConversationSidebar';

interface ChatPageProps {
  apiKey?: string;
  workspaceId?: string;
}

export default function ChatPage({ apiKey, workspaceId }: ChatPageProps) {
  const [currentConversationId, setCurrentConversationId] = useState<string | null>(null);
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const { upsertConversation } = useConversationManager();

  const handleConversationStart = useCallback((id: string) => {
    setCurrentConversationId(id);
    // This will be called when first message is sent
    // The actual title will be updated via upsertConversation
  }, []);

  const handleSelectConversation = useCallback((id: string) => {
    setCurrentConversationId(id);
  }, []);

  const handleNewConversation = useCallback(() => {
    setCurrentConversationId(null);
  }, []);

  // Update conversation in sidebar when message is sent
  const handleConversationUpdate = useCallback((id: string, message: string) => {
    upsertConversation(id, message);
  }, [upsertConversation]);

  return (
    <div className="flex h-full">
      {/* Mobile Sidebar Toggle */}
      <button
        onClick={() => setSidebarOpen(!sidebarOpen)}
        className="lg:hidden fixed top-4 left-4 z-50 p-2 bg-gray-800 rounded-lg text-gray-400 hover:text-white"
      >
        <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          {sidebarOpen ? (
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
          ) : (
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
          )}
        </svg>
      </button>

      {/* Sidebar */}
      <div
        className={`${
          sidebarOpen ? 'translate-x-0' : '-translate-x-full'
        } lg:translate-x-0 fixed lg:relative z-40 w-64 h-full transition-transform duration-300 ease-in-out`}
      >
        <ConversationSidebar
          currentConversationId={currentConversationId}
          onSelectConversation={handleSelectConversation}
          onNewConversation={handleNewConversation}
        />
      </div>

      {/* Overlay for mobile */}
      {sidebarOpen && (
        <div
          className="lg:hidden fixed inset-0 bg-black/50 z-30"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      {/* Main Chat Area */}
      <div className="flex-1 flex flex-col h-full">
        <StreamingChat
          key={currentConversationId || 'new'}
          conversationId={currentConversationId || undefined}
          workspaceId={workspaceId}
          apiKey={apiKey}
          onConversationStart={(id) => {
            handleConversationStart(id);
            // Also update the sidebar with the first message
            handleConversationUpdate(id, 'New conversation');
          }}
        />
      </div>
    </div>
  );
}
