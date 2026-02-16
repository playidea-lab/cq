import React from 'react';

interface MainLayoutProps {
  sidebar: React.ReactNode;
  header: React.ReactNode;
  content: React.ReactNode;
  drawer?: React.ReactNode;
  isDrawerOpen?: boolean;
}

export function MainLayout({ 
  sidebar, 
  header, 
  content, 
  drawer, 
  isDrawerOpen 
}: MainLayoutProps) {
  return (
    <div className="app-layout">
      {sidebar}
      <main className="app-main">
        {header}
        <div className="app-container">
          <div className="app-content-wrapper">
            {content}
          </div>
          {drawer && (
            <aside className={`app-drawer ${isDrawerOpen ? 'app-drawer--open' : ''}`}>
              {drawer}
            </aside>
          )}
        </div>
      </main>
    </div>
  );
}
