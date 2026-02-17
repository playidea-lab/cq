import React from 'react';

interface MainLayoutProps {
  sidebar: React.ReactNode;
  header: React.ReactNode;
  content: React.ReactNode;
  messenger?: React.ReactNode;
  isMessengerOpen?: boolean;
}

export function MainLayout({
  sidebar,
  header,
  content,
  messenger,
  isMessengerOpen = true
}: MainLayoutProps) {
  return (
    <div className="app-layout">
      {sidebar}
      <main className="app-main">
        {header}
        <div className="app-container">
          {/* Main content: 100% or 50% depending on messenger state */}
          <div className={`app-content ${isMessengerOpen && messenger ? 'app-content--split' : 'app-content--full'}`}>
            {content}
          </div>
          {/* Messenger: toggleable */}
          {messenger && isMessengerOpen && (
            <aside className="app-messenger-fixed">
              {messenger}
            </aside>
          )}
        </div>
      </main>
    </div>
  );
}
