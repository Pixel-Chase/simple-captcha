import { createRoot } from 'react-dom/client';
import { ConsoleApp } from './ConsoleApp';
import '../styles.css';

const container = document.getElementById('root');
if (!container) {
  throw new Error('console: 找不到 #root 容器');
}

createRoot(container).render(<ConsoleApp />);
