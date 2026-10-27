import { useId, useState } from 'react';
import type { ReactNode } from 'react';
import { CaptchaError, useCaptcha } from '@aetherlib/g-captcha';
import type { CaptchaType } from '@aetherlib/g-captcha';
import { getErrorMessage } from '../api';
import { Icon } from '../components/Icon';
import type { IconName } from '../components/Icon';
import './GuestPages.css';

interface TryEntry {
  type: CaptchaType;
  label: string;
  /** 完整版按钮里的图标；只有点选的两个次要按钮带 */
  icon?: IconName;
}

interface Tile {
  id: string;
  title: string;
  icon: IconName;
  desc: string;
  art: ReactNode;
  tries: ReadonlyArray<TryEntry>;
}

type TryTone = 'ok' | 'fail' | 'cancel';

interface TryResult {
  tone: TryTone;
  /** 失败原因，放在结果标签的悬停提示里 */
  detail?: string;
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

/** 未登录页的题型体验：示意图只是装饰，点击按钮弹出 SDK 的真实验证弹窗 */
export function CaptchaShowcase({ variant = 'full' }: CaptchaShowcaseProps) {
  const { verify, holder } = useCaptcha();
  const titleId = useId();
  const [pending, setPending] = useState<CaptchaType | null>(null);
  const [results, setResults] = useState<Partial<Record<string, TryResult>>>({});

  const compact = variant === 'compact';

  const tryType = async (tileId: string, type: CaptchaType) => {
    if (pending) {
      return;
    }
    setPending(type);
    let result: TryResult;
    try {
      await verify({ type });
      result = { tone: 'ok' };
    } catch (err) {
      result =
        err instanceof CaptchaError && err.code === 'CANCELLED'
          ? { tone: 'cancel' }
          : { tone: 'fail', detail: getErrorMessage(err, '验证未完成') };
    }
    setResults((prev) => ({ ...prev, [tileId]: result }));
    setPending(null);
  };

  const renderResult = (result: TryResult | undefined) => {
    if (!result) {
      return compact ? null : <span className="showcase-card__none">—</span>;
    }
    const view = RESULT_VIEW[result.tone];
    return (
      <span className={view.chip} title={result.detail}>
        {compact ? (
          <span className="chip__dot" aria-hidden="true" />
        ) : (
          <Icon name={view.icon} size={13} />
        )}
        {view.text}
      </span>
    );
  };

  const renderActions = (tile: Tile) => (
    <div className="showcase-card__actions">
      {tile.tries.map((item) => {
        const running = pending === item.type;
        // 完整版：多个入口用浅色次要按钮，单个入口用主按钮；紧凑版统一为胶囊次要按钮
        let className = 'btn showcase__try';
        if (!compact) {
          className += tile.tries.length > 1 ? ' showcase__try--tint' : ' btn--primary';
        }
        const icon: IconName | undefined = running
          ? 'spinner'
          : compact
            ? undefined
            : (item.icon ?? 'play');
        return (
          <button
            key={item.type}
            type="button"
            className={className}
            disabled={pending !== null}
            onClick={() => tryType(tile.id, item.type)}
          >
            {icon && <Icon name={icon} size={15} className={running ? 'spin' : undefined} />}
            {running ? '验证中…' : item.label}
          </button>
        );
      })}
    </div>
  );

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
            <p className="showcase__intro">无需登录即可试用四类题型，熟悉每种题型的交互方式。</p>
          )}
        </div>
        {compact ? (
          <p className="showcase__intro">无需登录即可试用四类题型，点击按钮会弹出真实的验证码。</p>
        ) : (
          <span className="showcase__note">
            <Icon name="info" size={14} />
            点击后弹出真实验证码，卡片中的图示仅为示意
          </span>
        )}
      </div>

      <ul className="showcase__grid">
        {TILES.map((tile, index) => {
          const no = String(index + 1).padStart(2, '0');
          const result = results[tile.id];

          if (compact) {
            return (
              <li key={tile.id} className="showcase-card showcase-card--compact">
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
                {renderActions(tile)}
              </li>
            );
          }

          return (
            <li key={tile.id} className="showcase-card">
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
                  {renderActions(tile)}
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

/** 点选：已点过的字被序号圆点盖住，「信」是下一个目标，其余为干扰字 */
const CLICK_CELLS: ReadonlyArray<{ key: string; mark?: number; next?: boolean }> = [
  { key: '云' },
  { key: '智', mark: 1 },
  { key: '光' },
  { key: '慧', mark: 2 },
  { key: '信', next: true },
];

function ClickArt() {
  return (
    <div className="showcase-art" aria-hidden="true">
      <span className="showcase-art__prompt">
        请依序点选：<b>智</b> · <b>慧</b> · <b>信</b>
      </span>
      <div className="showcase-art__board">
        {CLICK_CELLS.map((cell) =>
          cell.mark ? (
            <span key={cell.key} className="showcase-art__mark">
              {cell.mark}
            </span>
          ) : (
            <span
              key={cell.key}
              className={
                cell.next ? 'showcase-art__glyph showcase-art__glyph--next' : 'showcase-art__glyph'
              }
            >
              {cell.key}
            </span>
          ),
        )}
      </div>
    </div>
  );
}

function SlideArt() {
  return (
    <div className="showcase-art" aria-hidden="true">
      <div className="showcase-art__strip">
        <span className="showcase-art__piece">
          <Icon name="puzzle" size={14} />
        </span>
        <span className="showcase-art__dash" />
        <span className="showcase-art__slot" />
      </div>
      <div className="showcase-art__track">
        <span className="showcase-art__knob" />
      </div>
    </div>
  );
}

function RegionArt() {
  return (
    <div className="showcase-art" aria-hidden="true">
      <div className="showcase-art__panel">
        <svg className="showcase-art__path" viewBox="0 0 176 96" aria-hidden="true">
          <path d="M38 20Q104 10 146 62" />
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

function RotateArt() {
  return (
    <div className="showcase-art" aria-hidden="true">
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
    id: 'click',
    title: '点选验证',
    icon: 'pointer',
    desc: '按提示顺序点击图中对应的文字或图形。',
    art: <ClickArt />,
    tries: [
      { type: 'click-text', label: '文字点选', icon: 'file-text' },
      { type: 'click-shape', label: '图形点选', icon: 'grid' },
    ],
  },
  {
    id: 'slide',
    title: '滑块拼图',
    icon: 'puzzle',
    desc: '向右拖动滑块，把拼图块嵌进目标缺口。',
    art: <SlideArt />,
    tries: [{ type: 'slide-text', label: '体验滑块拼图' }],
  },
  {
    id: 'region',
    title: '空间拼图',
    icon: 'move',
    desc: '按住图块在画面内自由拖动，放进缺口后松手即提交。',
    art: <RegionArt />,
    tries: [{ type: 'slide-region', label: '体验空间拼图' }],
  },
  {
    id: 'rotate',
    title: '旋转验证',
    icon: 'rotate',
    desc: '拖动滑块旋转倾斜的圆形图片，把它转回正向。',
    art: <RotateArt />,
    tries: [{ type: 'rotate', label: '体验旋转验证' }],
  },
];
