import Header from './components/Header';
import Chat from './components/Chat';

export default function Home() {
  return (
    <div className="flex flex-col h-screen bg-gray-900">
      <Header />
      <main className="flex-1 max-w-4xl w-full mx-auto">
        <Chat />
      </main>
    </div>
  );
}
