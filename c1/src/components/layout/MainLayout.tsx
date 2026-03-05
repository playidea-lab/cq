import React from 'react';

interface MainLayoutProps {
  leftNav: React.ReactNode;
  channelList: React.ReactNode;
  content: React.ReactNode;
}

export function MainLayout({
  leftNav,
  channelList,
  content,
}: MainLayoutProps) {
  return (
    <div className="main-layout">
      {/* Left nav: 48px fixed */}
      <div className="main-layout__nav">
        {leftNav}
      </div>
      {/* Channel list area: 240px — hidden when null */}
      {channelList != null && (
        <div className="main-layout__channel-list">
          {channelList}
        </div>
      )}
      {/* Content area: flex-grow */}
      <div className="main-layout__content">
        {content}
      </div>
    </div>
  );
}
