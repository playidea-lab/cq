import Header from './components/Header';
import ChatPage from './components/ChatPage';

export default function Home() {
  // API key can be passed via environment variable or user input
  const apiKey = process.env.NEXT_PUBLIC_C4_API_KEY;

  return (
    <div className="flex flex-col h-screen bg-gray-900">
      <Header />
      <main className="flex-1 overflow-hidden">
        <ChatPage apiKey={apiKey} />
      </main>
    </div>
  );
}
