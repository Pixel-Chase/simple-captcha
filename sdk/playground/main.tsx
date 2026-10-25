import { createRoot } from 'react-dom/client';
import App from './App';
import './styles.css';

const container = document.getElementById('root');
if (!container) {
  throw new Error('playground: 找不到 #root 容器');
}

createRoot(container).render(<App />);
