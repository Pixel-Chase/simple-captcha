import { useId, useRef, useState } from 'react';
import type { PointerEvent, ReactNode } from 'react';
import { CaptchaError, useCaptcha } from '@aetherlib/g-captcha';
import type { CaptchaType } from '@aetherlib/g-captcha';
import { getErrorMessage } from '../api';
import { Icon } from '../components/Icon';
import type { IconName } from '../components/Icon';
import './GuestPages.css';

/** 每个题型一张卡片、一个体验按钮 */
interface Tile {
  type: CaptchaType;
  title: string;
  icon: IconName;
  /** 一行放得下：五列布局下卡片正文约 15 个字宽，控制在 14 个字以内 */
  desc: string;
  art: ReactNode;
}

type TryTone = 'ok' | 'fail' | 'cancel';

interface TryResult {
  tone: TryTone;
  /** 失败原因，放在结果标签的悬停提示里 */
  detail?: string;
  /** 每次体验递增，用作结果标签与卡片闪光的 key：同样的结果再来一次也会重播动效 */
  seq: number;
}

const RESULT_VIEW: Record<TryTone, { text: string; chip: string; icon: IconName }> = {
  ok: { text: '验证通过', chip: 'chip chip--good', icon: 'check-circle' },
  fail: { text: '验证未完成', chip: 'chip chip--warn', icon: 'error' },
  cancel: { text: '你已取消本次验证', chip: 'chip', icon: 'x-circle' },
};

interface CaptchaShowcaseProps {
  /** full：登录页，带示意图与「最近体验」页脚；compact：初始化页，无示意图，结果在卡片右上角 */
  variant?: 'full' | 'compact';
}

/** 卡片聚光：把指针在卡片内的位置写进 CSS 变量，由卡片 ::before 的径向渐变跟随 */
function moveSpotlight(event: PointerEvent<HTMLUListElement>) {
  const card = (event.target as Element).closest<HTMLElement>('.showcase-card');
  if (!card) {
    return;
  }
  const rect = card.getBoundingClientRect();
  card.style.setProperty('--spot-x', `${event.clientX - rect.left}px`);
  card.style.setProperty('--spot-y', `${event.clientY - rect.top}px`);
}

/** 未登录页的题型体验：示意图只是装饰（循环演示交互过程），点击按钮弹出 SDK 的真实验证弹窗 */
export function CaptchaShowcase({ variant = 'full' }: CaptchaShowcaseProps) {
  const { verify, holder } = useCaptcha();
  const titleId = useId();
  const [pending, setPending] = useState<CaptchaType | null>(null);
  const [results, setResults] = useState<Partial<Record<CaptchaType, TryResult>>>({});
  const seqRef = useRef(0);

  const compact = variant === 'compact';

  const tryType = async (type: CaptchaType) => {
    if (pending) {
      return;
    }
    setPending(type);
    seqRef.current += 1;
    const seq = seqRef.current;
    let result: TryResult;
    try {
      await verify({ type });
      result = { tone: 'ok', seq };
    } catch (err) {
      result =
        err instanceof CaptchaError && err.code === 'CANCELLED'
          ? { tone: 'cancel', seq }
          : { tone: 'fail', detail: getErrorMessage(err, '验证未完成'), seq };
    }
    setResults((prev) => ({ ...prev, [type]: result }));
    setPending(null);
  };

  const renderResult = (result: TryResult | undefined) => {
    if (!result) {
      return compact ? null : <span className="showcase-card__none">—</span>;
    }
    const view = RESULT_VIEW[result.tone];
    return (
      <span key={result.seq} className={view.chip} title={result.detail}>
        {compact ? (
          <span className="chip__dot" aria-hidden="true" />
        ) : (
          <Icon name={view.icon} size={13} />
        )}
        {view.text}
      </span>
    );
  };

  // 完整版是主按钮，紧凑版是胶囊次要按钮
  const renderAction = (tile: Tile) => {
    const running = pending === tile.type;
    const icon: IconName | undefined = running ? 'spinner' : compact ? undefined : 'play';
    return (
      <div className="showcase-card__actions">
        <button
          type="button"
          className={compact ? 'btn showcase__try' : 'btn btn--primary showcase__try'}
          disabled={pending !== null}
          onClick={() => tryType(tile.type)}
        >
          {icon && <Icon name={icon} size={15} className={running ? 'spin' : undefined} />}
          {running ? '验证中…' : `体验${tile.title}`}
        </button>
      </div>
    );
  };

  return (
    <section
      className={compact ? 'showcase showcase--compact' : 'showcase'}
      aria-labelledby={titleId}
    >
      <div className="captcha-holder">{holder}</div>

      <div className="showcase__head">
        <div>
          <span className="showcase__eyebrow">
            <Icon name="pointer" size={16} />
            交互体验
          </span>
          <h2 id={titleId} className="showcase__title">
            题型实时体验
          </h2>
          {!compact && (
            <p className="showcase__intro">无需登录即可试用五种题型，熟悉每种题型的交互方式。</p>
          )}
        </div>
        {compact ? (
          <p className="showcase__intro">无需登录即可试用五种题型，点击按钮会弹出真实的验证码。</p>
        ) : (
          <span className="showcase__note">
            <Icon name="info" size={14} />
            点击后弹出真实验证码，卡片中的图示仅为示意
          </span>
        )}
      </div>

      <ul className="showcase__grid" onPointerMove={moveSpotlight}>
        {TILES.map((tile, index) => {
          const no = String(index + 1).padStart(2, '0');
          const result = results[tile.type];
          // 通过 / 失败时卡片闪一下；取消是用户主动操作，不打扰
          const flash = result && result.tone !== 'cancel' && (
            <span
              key={result.seq}
              className={`showcase-card__flash showcase-card__flash--${result.tone}`}
              aria-hidden="true"
            />
          );

          if (compact) {
            return (
              <li key={tile.type} className="showcase-card showcase-card--compact">
                {flash}
                <div className="showcase-card__main">
                  <div className="showcase-card__top">
                    <span className="showcase-card__num">{no}</span>
                    <span className="showcase-card__result" role="status">
                      {renderResult(result)}
                    </span>
                  </div>
                  <h3 className="showcase-card__name">{tile.title}</h3>
                  <p className="showcase-card__desc">{tile.desc}</p>
                </div>
                {renderAction(tile)}
              </li>
            );
          }

          return (
            <li key={tile.type} className="showcase-card">
              {flash}
              <div className="showcase-card__main">
                <div className="showcase-card__top">
                  <span className="showcase-card__tag">
                    {no} · {tile.title}
                  </span>
                  <Icon name={tile.icon} size={18} />
                </div>
                {tile.art}
                <div>
                  <h3 className="showcase-card__name">{tile.title}</h3>
                  <p className="showcase-card__desc">{tile.desc}</p>
                </div>
                <div className="showcase-card__entry">
                  <span className="showcase-card__label">体验入口：</span>
                  {renderAction(tile)}
                </div>
              </div>
              <div className="showcase-card__foot">
                <span className="showcase-card__label">最近体验：</span>
                <span className="showcase-card__result" role="status">
                  {renderResult(result)}
                </span>
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

/* ---------- 示意图（纯装饰，读屏跳过） ---------- */
/*
 * 每张示意图循环演示一次完整交互（约 6 秒一轮，五张卡片错开），动画全在 GuestPages.css；
 * 关闭动画（减少动态效果）时停在静态示意：点选显示点完的样子，其余显示初始位置
 */

type Shape = 'triangle' | 'circle' | 'square' | 'diamond' | 'star';

const SHAPE_PATHS: Record<Shape, ReactNode> = {
  triangle: <path d="M8 2.5 14 13.5H2Z" />,
  circle: <circle cx="8" cy="8" r="5.5" />,
  square: <rect x="2.5" y="2.5" width="11" height="11" rx="1.5" />,
  diamond: <path d="M8 1.5 14.5 8 8 14.5 1.5 8Z" />,
  star: <path d="M8 1.8l1.9 3.9 4.3.6-3.1 3 .7 4.3L8 11.6l-3.8 2 .7-4.3-3.1-3 4.3-.6z" />,
};

function ShapeGlyph({ shape, size }: { shape: Shape; size: number }) {
  return (
    <svg
      className="showcase-art__shape"
      viewBox="0 0 16 16"
      width={size}
      height={size}
      aria-hidden="true"
    >
      {SHAPE_PATHS[shape]}
    </svg>
  );
}

interface ClickCell {
  key: string;
  content: ReactNode;
  /** 目标的点选序号；没有的是干扰项 */
  mark?: number;
}

/** 点选：提示里的三个目标依次被序号圆点盖住，其余为干扰项；文字点选与图形点选共用 */
function ClickArt({ prompt, cells }: { prompt: ReactNode; cells: ReadonlyArray<ClickCell> }) {
  return (
    <div className="showcase-art showcase-art--click" aria-hidden="true">
      <span className="showcase-art__prompt">请依序点选：{prompt}</span>
      <div className="showcase-art__board">
        {cells.map((cell) => (
          <span key={cell.key} className="showcase-art__cell">
            <span className="showcase-art__glyph">{cell.content}</span>
            {cell.mark && (
              <span className={`showcase-art__mark showcase-art__mark--${cell.mark}`}>
                {cell.mark}
              </span>
            )}
          </span>
        ))}
      </div>
    </div>
  );
}

const TEXT_CELLS: ReadonlyArray<ClickCell> = [
  { key: '云', content: '云' },
  { key: '智', content: '智', mark: 1 },
  { key: '光', content: '光' },
  { key: '慧', content: '慧', mark: 2 },
  { key: '信', content: '信', mark: 3 },
];

const SHAPE_CELLS: ReadonlyArray<ClickCell> = (
  [['star'], ['triangle', 1], ['diamond'], ['circle', 2], ['square', 3]] as const
).map(([shape, mark]) => ({ key: shape, content: <ShapeGlyph shape={shape} size={14} />, mark }));

const TextClickArt = () => (
  <ClickArt
    prompt={
      <>
        <b>智</b> · <b>慧</b> · <b>信</b>
      </>
    }
    cells={TEXT_CELLS}
  />
);

const ShapeClickArt = () => (
  <ClickArt
    prompt={
      <>
        <b>
          <ShapeGlyph shape="triangle" size={10} />
        </b>{' '}
        ·{' '}
        <b>
          <ShapeGlyph shape="circle" size={10} />
        </b>{' '}
        ·{' '}
        <b>
          <ShapeGlyph shape="square" size={10} />
        </b>
      </>
    }
    cells={SHAPE_CELLS}
  />
);

/** 滑块拼图：滑块拖到底，拼图块略微冲过缺口再回正，缺口亮绿 */
function SlideArt() {
  return (
    <div className="showcase-art showcase-art--slide" aria-hidden="true">
      <div className="showcase-art__strip">
        <span className="showcase-art__piece">
          <Icon name="puzzle" size={14} />
        </span>
        <span className="showcase-art__dash" />
        <span className="showcase-art__slot" />
      </div>
      <div className="showcase-art__track">
        <span className="showcase-art__fill" />
        <span className="showcase-art__knob" />
      </div>
    </div>
  );
}

/** 空间拼图：图块被拿起、沿虚线轨迹飞进右下角的缺口；虚线从图块中心连到缺口中心 */
function RegionArt() {
  return (
    <div className="showcase-art showcase-art--region" aria-hidden="true">
      <div className="showcase-art__panel">
        <svg className="showcase-art__path" viewBox="0 0 160 96" aria-hidden="true">
          <path d="M24 20Q100 4 132 76" />
        </svg>
        <span className="showcase-art__piece">
          <Icon name="move" size={14} />
        </span>
        <span className="showcase-art__slot" />
      </div>
    </div>
  );
}

/** 刻度环：24 格，每 90° 一根长刻度；12 点方向用圆点代替，表示正向基准 */
const DIAL_TICKS = Array.from({ length: 24 }, (_, i) => i).filter((i) => i !== 0);

/** 旋转：倾斜 45° 的指针转回正向（带一点回弹），基准点亮绿 */
function RotateArt() {
  return (
    <div className="showcase-art showcase-art--rotate" aria-hidden="true">
      <div className="showcase-art__dial">
        <svg
          className="showcase-art__ticks"
          viewBox="0 0 88 88"
          width="88"
          height="88"
          aria-hidden="true"
        >
          <circle className="showcase-art__face" cx="44" cy="44" r="40" />
          {DIAL_TICKS.map((i) => (
            <line
              key={i}
              className={
                i % 6 === 0 ? 'showcase-art__tick showcase-art__tick--major' : 'showcase-art__tick'
              }
              x1="44"
              y1="4"
              x2="44"
              y2={i % 6 === 0 ? 10 : 8}
              transform={`rotate(${i * 15} 44 44)`}
            />
          ))}
          <circle className="showcase-art__zero" cx="44" cy="5" r="3" />
          <circle className="showcase-art__disc" cx="44" cy="44" r="28" />
        </svg>
        <Icon name="compass" size={26} className="showcase-art__needle" />
      </div>
    </div>
  );
}

const TILES: ReadonlyArray<Tile> = [
  {
    type: 'click-text',
    title: '文字点选',
    icon: 'file-text',
    desc: '按提示顺序点击图中的文字。',
    art: <TextClickArt />,
  },
  {
    type: 'click-shape',
    title: '图形点选',
    icon: 'grid',
    desc: '按提示顺序点击图中的图形。',
    art: <ShapeClickArt />,
  },
  {
    type: 'slide-text',
    title: '滑块拼图',
    icon: 'puzzle',
    desc: '拖动滑块，把拼图块嵌进缺口。',
    art: <SlideArt />,
  },
  {
    type: 'slide-region',
    title: '空间拼图',
    icon: 'move',
    desc: '按住图块拖进缺口，松手即验。',
    art: <RegionArt />,
  },
  {
    type: 'rotate',
    title: '旋转验证',
    icon: 'rotate',
    desc: '拖动滑块，把图片转回正向。',
    art: <RotateArt />,
  },
];
