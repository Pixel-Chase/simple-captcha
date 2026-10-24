import { useEffect, useState } from 'react';
import { DEMO_ID, DOC_ANCHOR_IDS, DOC_GROUPS, README_URL, docsHash } from '../docs/content';
import { LiveDemo } from '../docs/LiveDemo';

interface DocsPageProps {
  /** hash 路由 #/docs/<anchor> 中的锚点，为空时停留在当前位置 */
  anchor: string | null;
}

/** 顶栏是 sticky 的，标题在视口里越过这条线才算"当前章节" */
const ACTIVE_LINE = 104;

const ALL_SECTIONS = DOC_GROUPS.flatMap((group) => group.sections);

/** 标题自身带 scroll-margin-top，scrollIntoView 会自动避开顶栏 */
function scrollToId(id: string) {
  document.getElementById(id)?.scrollIntoView({ block: 'start' });
}

/** 滚动时找出最后一个越过 ACTIVE_LINE 的标题；ids 按文档顺序排列，所以可以提前退出 */
function useActiveAnchor(ids: string[]) {
  const [active, setActive] = useState(ids[0]);

  useEffect(() => {
    let frame = 0;
    const update = () => {
      frame = 0;
      let current = ids[0];
      for (const id of ids) {
        const element = document.getElementById(id);
        if (!element) {
          continue;
        }
        if (element.getBoundingClientRect().top > ACTIVE_LINE) {
          break;
        }
        current = id;
      }
      setActive(current);
    };
    const schedule = () => {
      if (!frame) {
        frame = requestAnimationFrame(update);
      }
    };

    update();
    window.addEventListener('scroll', schedule, { passive: true });
    window.addEventListener('resize', schedule);
    return () => {
      window.removeEventListener('scroll', schedule);
      window.removeEventListener('resize', schedule);
      if (frame) {
        cancelAnimationFrame(frame);
      }
    };
  }, [ids]);

  return active;
}

export function DocsPage({ anchor }: DocsPageProps) {
  const active = useActiveAnchor(DOC_ANCHOR_IDS);

  // 内容是同步渲染的静态 HTML，首个 effect 执行时目标节点已经存在
  useEffect(() => {
    if (anchor) {
      scrollToId(anchor);
    }
  }, [anchor]);

  const activeSectionId = ALL_SECTIONS.find(
    (section) => section.id === active || section.children.some((item) => item.id === active),
  )?.id;

  const linkClass = (id: string, current = false) =>
    [
      'docs__link',
      id === active && 'docs__link--active',
      current && id !== active && 'docs__link--current',
    ]
      .filter(Boolean)
      .join(' ');

  // href 负责改 hash（可分享、可后退），onClick 兜住 hash 未变化时的重复点击
  const navLink = (id: string, title: string, current = false) => (
    <a className={linkClass(id, current)} href={docsHash(id)} onClick={() => scrollToId(id)}>
      {title}
    </a>
  );

  return (
    <div className="docs">
      <aside className="docs__nav" aria-label="文档目录">
        {navLink(DEMO_ID, '在线体验')}
        {DOC_GROUPS.map((group) => (
          <div key={group.title} className="docs__group">
            <div className="docs__group-title">{group.title}</div>
            {group.sections.map((section) => (
              <div key={section.id}>
                {navLink(section.id, section.title, activeSectionId === section.id)}
                {section.children.length > 0 && (
                  <div className="docs__sub">
                    {section.children.map((child) => (
                      <span key={child.id}>{navLink(child.id, child.title)}</span>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        ))}
      </aside>

      <div className="docs__body">
        <header className="docs__head">
          <p className="eyebrow">接入文档</p>
          <h1>把行为验证码接进你的系统</h1>
          <p className="muted">
            前端装 SDK 弹窗验证拿到 <code>second_key</code>，业务后端再拿它向验证码服务做二次确认。
            以下章节实时渲染自仓库根目录的{' '}
            <a href={README_URL} target="_blank" rel="noreferrer">
              README.md
            </a>
            ，与 GitHub 上完全一致；修订文档请直接改 README。
          </p>
        </header>

        <LiveDemo />

        {ALL_SECTIONS.map((section) => (
          <article
            key={section.id}
            className="docs__section markdown"
            // biome-ignore lint/security/noDangerouslySetInnerHtml: 内容来自仓库自身的 README，不含用户输入
            dangerouslySetInnerHTML={{ __html: section.html }}
          />
        ))}
      </div>
    </div>
  );
}
