import { createRoot } from 'react-dom/client';
// 全局样式先于页面组件引入，页面自己的 CSS 才能按层叠顺序覆盖通用类
import './styles.css';
import App from './App';

const container = document.getElementById('root');
if (!container) {
  throw new Error('playground: 找不到 #root 容器');
}

createRoot(container).render(<App />);
