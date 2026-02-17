import { ChannelsView } from './ChannelsView';

interface ChatDrawerProps {
  projectPath: string;
}

export function ChatDrawer({ projectPath }: ChatDrawerProps) {
  return (
    <div className="chat-drawer-container">
      <div className="chat-drawer-header">
        <h3>Messenger</h3>
      </div>
      <div className="chat-drawer-body">
        <ChannelsView projectPath={projectPath} />
      </div>
    </div>
  );
}