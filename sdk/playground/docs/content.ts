import { Marked } from 'marked';
import type { Token, Tokens } from 'marked';
import readme from '../../../README.md?raw';

/** 仓库文件在 GitHub 上的地址，README 里的相对链接统一改写到这里 */
const REPO_BLOB_BASE = 'https://github.com/Pixel-Chase/simple-captcha/blob/main/';
export const README_URL = `${REPO_BLOB_BASE}README.md`;

/** 文档页内的锚点一律写成 #/docs/<id>，避免与 hash 路由冲突 */
export const DOCS_HASH_PREFIX = '#/docs/';
export const docsHash = (id: string) => `${DOCS_HASH_PREFIX}${id}`;

export interface DocHeading {
  id: string;
  /** 去掉 Markdown 标记后的纯文本标题 */
  title: string;
}

export interface DocSection extends DocHeading {
  html: string;
  /** 本章节下的三级标题，用于侧栏二级目录 */
  children: DocHeading[];
}

export interface DocGroup {
  title: string;
  sections: DocSection[];
}

/**
 * 页面展示的 README 章节，按接入视角重排、分组。
 * 名称必须与 README 的二级标题逐字一致；README 改了标题这里要同步，否则该章节会被跳过并在控制台告警。
 */
const CURATED_GROUPS: ReadonlyArray<{ title: string; headings: string[] }> = [
  { title: '概览', headings: ['架构概览', '验证流程'] },
  { title: '前端接入', headings: ['前端接入', 'API 参考'] },
  {
    title: '后端接入',
    headings: ['后端部署', '二次验证（S2S）', '管理后台', '环境变量参考', '安全机制'],
  },
];

/** 分词阶段给标题 token 挂上 id，渲染阶段直接读取 */
type HeadingToken = Tokens.Heading & { id?: string };

interface PreparedSection {
  tokens: Token[];
  headings: Array<DocHeading & { depth: number }>;
}

/** 与 GitHub 相同的标题锚点规则：小写、去标点、空格转连字符、重复标题追加序号 */
function createSlugger() {
  const seen = new Map<string, number>();
  return (text: string) => {
    const base = text
      .trim()
      .toLowerCase()
      .replace(/[^\p{L}\p{N}\s-]/gu, '')
      .replace(/\s/g, '-');
    const count = seen.get(base) ?? 0;
    seen.set(base, count + 1);
    return count === 0 ? base : `${base}-${count}`;
  };
}

const plainText = (markdown: string) => markdown.replace(/[`*_]/g, '').trim();

const escapeHtml = (value: string) =>
  value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

/** 按二级标题切分 README，代码块内的 `##` 不算标题 */
function splitByH2(markdown: string): Map<string, string> {
  const sections = new Map<string, string[]>();
  let current: string[] | null = null;
  let inFence = false;

  for (const line of markdown.split('\n')) {
    if (/^\s*(```|~~~)/.test(line)) {
      inFence = !inFence;
    } else if (!inFence) {
      const match = /^##\s+(.+?)\s*$/.exec(line);
      if (match) {
        current = [];
        sections.set(match[1], current);
        continue;
      }
    }
    current?.push(line);
  }

  return new Map([...sections].map(([title, lines]) => [title, lines.join('\n').trim()]));
}

/** 宽表格外面包一层可横向滚动的容器 */
const wrapTables = (html: string) =>
  html
    .replace(/<table>/g, '<div class="markdown__table"><table>')
    .replace(/<\/table>/g, '</table></div>');

/**
 * 自定义渲染：
 * - 标题带 id 与锚点链接
 * - README 内部锚点改写为 hash 路由；指向未展示章节的锚点和相对路径改写为 GitHub 链接
 * - 代码块带语言标签
 */
function createMarked(knownIds: Set<string>) {
  const rewriteHref = (href: string) => {
    if (href.startsWith('#')) {
      const slug = decodeURIComponent(href.slice(1));
      return knownIds.has(slug) ? docsHash(slug) : `${README_URL}#${slug}`;
    }
    if (/^[a-z][a-z0-9+.-]*:/i.test(href)) {
      return href;
    }
    return `${REPO_BLOB_BASE}${href.replace(/^\.?\//, '')}`;
  };

  return new Marked({
    gfm: true,
    renderer: {
      heading(token: HeadingToken) {
        const text = this.parser.parseInline(token.tokens);
        const tag = `h${token.depth}`;
        if (!token.id) {
          return `<${tag}>${text}</${tag}>\n`;
        }
        const anchor = `<a class="markdown__anchor" href="${docsHash(token.id)}" aria-label="本节链接">#</a>`;
        return `<${tag} id="${token.id}">${text}${anchor}</${tag}>\n`;
      },
      link(token) {
        const href = rewriteHref(token.href);
        const text = this.parser.parseInline(token.tokens);
        const title = token.title ? ` title="${escapeHtml(token.title)}"` : '';
        const external = /^https?:/.test(href) ? ' target="_blank" rel="noreferrer"' : '';
        return `<a href="${escapeHtml(href)}"${title}${external}>${text}</a>`;
      },
      code({ text, lang, escaped }) {
        const language = (lang || '').trim().split(/\s+/)[0];
        const body = escaped ? text : escapeHtml(text);
        const label = language ? `<span class="markdown__lang">${escapeHtml(language)}</span>` : '';
        const cls = language ? ` class="language-${escapeHtml(language)}"` : '';
        return `<div class="markdown__code">${label}<pre><code${cls}>${body}</code></pre></div>\n`;
      },
    },
  });
}

function buildDocs(): DocGroup[] {
  const sectionsByTitle = splitByH2(readme);
  const slug = createSlugger();
  const knownIds = new Set<string>();
  const marked = createMarked(knownIds);

  // 第一遍：分词并为所有标题分配 id。链接改写需要知道页面上到底有哪些锚点，所以必须先于渲染完成
  const prepared = CURATED_GROUPS.map((group) => ({
    title: group.title,
    sections: group.headings.flatMap<PreparedSection>((heading) => {
      const body = sectionsByTitle.get(heading);
      if (body === undefined) {
        console.warn(`[docs] README 中找不到二级标题「${heading}」，已跳过该章节`);
        return [];
      }
      const tokens = marked.lexer(`## ${heading}\n\n${body}`);
      const headings: PreparedSection['headings'] = [];
      marked.walkTokens(tokens, (token) => {
        if (token.type !== 'heading') {
          return;
        }
        const headingToken = token as HeadingToken;
        headingToken.id = slug(headingToken.text);
        knownIds.add(headingToken.id);
        headings.push({
          id: headingToken.id,
          title: plainText(headingToken.text),
          depth: headingToken.depth,
        });
      });
      return [{ tokens, headings }];
    }),
  }));

  // 第二遍：渲染。首个标题即上面补进去的二级标题
  return prepared.map((group) => ({
    title: group.title,
    sections: group.sections.map(({ tokens, headings }) => {
      const [self, ...rest] = headings;
      return {
        id: self.id,
        title: self.title,
        html: wrapTables(marked.parser(tokens)),
        children: rest.filter((item) => item.depth === 3).map(({ id, title }) => ({ id, title })),
      };
    }),
  }));
}

export const DOC_GROUPS: DocGroup[] = buildDocs();

/** 页面上所有锚点 id，按出现顺序排列，供滚动高亮使用 */
export const DOC_ANCHOR_IDS: string[] = DOC_GROUPS.flatMap((group) =>
  group.sections.flatMap((section) => [section.id, ...section.children.map((item) => item.id)]),
);
