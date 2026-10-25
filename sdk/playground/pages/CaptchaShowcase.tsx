import { useId, useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';
import { CaptchaError, useCaptcha } from '@aetherlib/g-captcha';
import type { CaptchaType } from '@aetherlib/g-captcha';
import { getErrorMessage } from '../api';

interface Tile {
  id: string;
  title: string;
  mode: string;
  desc: string;
  art: ReactNode;
  tries: ReadonlyArray<{ type: CaptchaType; label: string }>;
}

interface TryResult {
  tone: 'ok' | 'fail' | 'cancel';
  text: string;
}

/** 首页题型卡片：示意图只是装饰，点击按钮弹出 SDK 的真实验证弹窗 */
export function CaptchaShowcase() {
  const { verify, holder } = useCaptcha();
  const [pending, setPending] = useState<CaptchaType | null>(null);
  const [results, setResults] = useState<Partial<Record<string, TryResult>>>({});

  const tryType = async (tileId: string, type: CaptchaType) => {
    if (pending) {
      return;
    }
    setPending(type);
    let result: TryResult;
    try {
      await verify({ type });
      result = { tone: 'ok', text: '验证通过' };
    } catch (err) {
      result =
        err instanceof CaptchaError && err.code === 'CANCELLED'
          ? { tone: 'cancel', text: '你已取消本次验证' }
          : { tone: 'fail', text: getErrorMessage(err, '验证未完成') };
    }
    setResults((prev) => ({ ...prev, [tileId]: result }));
    setPending(null);
  };

  return (
    <section className="showcase" aria-labelledby="showcase-title">
      <div className="captcha-holder">{holder}</div>

      <div className="showcase__head">
        <h2 id="showcase-title">题型体验</h2>
        <p className="showcase__intro">
          五种题型共用一套居中弹窗，点击卡片上的按钮即可走一遍真实的验证流程。
        </p>
      </div>

      <div className="showcase__grid">
        {TILES.map((tile, index) => {
          const result = results[tile.id];
          return (
            <article key={tile.id} className="showcase__tile">
              <div className="showcase__label">
                <span className="showcase__pill">
                  {String(index + 1).padStart(2, '0')} · {tile.title}
                </span>
                <span className="showcase__mode">{tile.mode}</span>
              </div>

              <div className="showcase__card">
                {tile.art}
                <p className="showcase__desc">{tile.desc}</p>
                <div className="showcase__foot">
                  <span
                    className={
                      result
                        ? `showcase__result showcase__result--${result.tone}`
                        : 'showcase__result'
                    }
                    role="status"
                  >
                    {result?.text}
                  </span>
                  <div className="showcase__actions">
                    {tile.tries.map((item) => (
                      <button
                        key={item.type}
                        type="button"
                        className="btn btn--sm showcase__try"
                        disabled={pending !== null}
                        onClick={() => tryType(tile.id, item.type)}
                      >
                        {pending === item.type ? '验证中…' : item.label}
                      </button>
                    ))}
                  </div>
                </div>
              </div>
            </article>
          );
        })}
      </div>
    </section>
  );
}

/* ---------- 示意图 ---------- */

/** 同页有多份 SVG，渐变、裁剪等 id 必须唯一；useId 的结果含特殊字符，去掉后再用于 url(#…) */
function useSvgId() {
  return `art${useId().replace(/[^a-zA-Z0-9]/g, '')}`;
}

/** 示意图共用的风景底图，300×200 与出图画布同比例；放在 defs 里，用 <use> 引用（拼图块需要再引用一次） */
function SceneDefs({ id }: { id: string }) {
  return (
    <>
      <linearGradient id={`${id}-sky`} x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" stopColor="#1d1660" />
        <stop offset="0.55" stopColor="#5b3fd0" />
        <stop offset="1" stopColor="#f2a9b9" />
      </linearGradient>
      <g id={id}>
        <rect width="300" height="200" fill={`url(#${id}-sky)`} />
        <circle cx="38" cy="30" r="1.4" fill="#fff" opacity="0.7" />
        <circle cx="120" cy="18" r="1" fill="#fff" opacity="0.6" />
        <circle cx="262" cy="26" r="1.2" fill="#fff" opacity="0.7" />
        <circle cx="214" cy="74" r="24" fill="#ffd9ad" opacity="0.92" />
        <path d="M0 132 52 94l44 26 54-50 56 46 44-28 50 24v88H0Z" fill="#4b3ea6" opacity="0.88" />
        <path
          d="M18 200v-52h16v-22h12v30h14v-40h18v48h12v-26h16v62Zm170 0v-46h14v-30h20v20h12v-34h16v50h14v40Z"
          fill="#2a2470"
        />
        <path d="M0 160 60 132l58 20 58-30 60 26 64-14v66H0Z" fill="#231f63" opacity="0.92" />
        <path d="M0 184q80-22 150-8t150-6v30H0Z" fill="#15133f" />
      </g>
    </>
  );
}

const CLICK_TARGETS = [
  { char: '安', x: 66, y: 140, rotate: -8 },
  { char: '全', x: 214, y: 120, rotate: 12 },
  { char: '验', x: 96, y: 62, rotate: 4 },
  { char: '证', x: 244, y: 56, rotate: -12 },
];

function ClickArt() {
  const id = useSvgId();
  const [first, second] = CLICK_TARGETS;
  return (
    <div className="art">
      <div className="art__frame">
        <svg className="art__scene" viewBox="0 0 300 200" role="img" aria-label="文字点选示意">
          <defs>
            <SceneDefs id={id} />
          </defs>
          <use href={`#${id}`} />
          <line className="art__guide" x1={first.x} y1={first.y} x2={second.x} y2={second.y} />
          {CLICK_TARGETS.map((item) => (
            <text
              key={item.char}
              className="art__char"
              x={item.x}
              y={item.y}
              transform={`rotate(${item.rotate} ${item.x} ${item.y})`}
            >
              {item.char}
            </text>
          ))}
          <text className="art__char art__char--decoy" x="158" y="96">
            盾
          </text>
          {[first, second].map((item, index) => (
            <g key={item.char}>
              <circle
                className="art__ping"
                cx={item.x - 14}
                cy={item.y - 14}
                r="10"
                style={{ animationDelay: `${index * 0.8}s` }}
              />
              <circle className="art__mark" cx={item.x - 14} cy={item.y - 14} r="10" />
              <text className="art__mark-num" x={item.x - 14} y={item.y - 14}>
                {index + 1}
              </text>
            </g>
          ))}
        </svg>
      </div>
      <div className="art__bar">
        <span>请依序点击</span>
        <span className="art__chips">
          {CLICK_TARGETS.map((item, index) => (
            <span
              key={item.char}
              className={
                index < 2
                  ? 'art__chip art__chip--done'
                  : index === 2
                    ? 'art__chip art__chip--next'
                    : 'art__chip'
              }
            >
              {item.char}
            </span>
          ))}
        </span>
      </div>
    </div>
  );
}

/** 拼图块：在 to 处引用一份底图，裁出 slot 位置的那一块 */
function Piece({
  id,
  slot,
  to,
}: {
  id: string;
  slot: { x: number; y: number };
  to: { x: number; y: number };
}) {
  return (
    <g transform={`translate(${to.x - slot.x} ${to.y - slot.y})`}>
      <use href={`#${id}`} clipPath={`url(#${id}-clip)`} />
      <rect className="art__piece" x={slot.x} y={slot.y} width="46" height="46" rx="9" />
    </g>
  );
}

function SlideArt() {
  const id = useSvgId();
  const slot = { x: 202, y: 74 };
  return (
    <div className="art">
      <div className="art__frame">
        <svg className="art__scene" viewBox="0 0 300 200" role="img" aria-label="滑块拼图示意">
          <defs>
            <SceneDefs id={id} />
            <clipPath id={`${id}-clip`}>
              <rect x={slot.x} y={slot.y} width="46" height="46" rx="9" />
            </clipPath>
          </defs>
          <use href={`#${id}`} />
          <rect className="art__slot" x={slot.x} y={slot.y} width="46" height="46" rx="9" />
          <g className="art__nudge">
            <Piece id={id} slot={slot} to={{ x: 84, y: slot.y }} />
          </g>
        </svg>
      </div>
      <Track progress={42} text="向右拖动滑块完成拼图" />
    </div>
  );
}

function RegionArt() {
  const id = useSvgId();
  const slot = { x: 222, y: 28 };
  const start = { x: 30, y: 118 };
  return (
    <div className="art">
      <div className="art__frame">
        <svg className="art__scene" viewBox="0 0 300 200" role="img" aria-label="空间拼图示意">
          <defs>
            <SceneDefs id={id} />
            <clipPath id={`${id}-clip`}>
              <rect x={slot.x} y={slot.y} width="46" height="46" rx="9" />
            </clipPath>
          </defs>
          <use href={`#${id}`} />
          <path className="art__guide" d="M53 141C100 146 160 108 245 51" />
          <rect className="art__slot" x={slot.x} y={slot.y} width="46" height="46" rx="9" />
          <g className="art__float">
            <Piece id={id} slot={slot} to={start} />
          </g>
        </svg>
      </div>
      <div className="art__bar">
        <span>按住图块拖入缺口</span>
        <span className="art__badge">松手即验</span>
      </div>
    </div>
  );
}

function RotateArt() {
  const id = useSvgId();
  return (
    <div className="art">
      <div className="art__frame art__frame--light">
        <svg className="art__scene" viewBox="0 0 300 200" role="img" aria-label="旋转验证示意">
          <defs>
            <SceneDefs id={id} />
            <clipPath id={`${id}-clip`}>
              <circle cx="150" cy="100" r="62" />
            </clipPath>
          </defs>
          <circle className="art__ring" cx="150" cy="100" r="80" />
          <circle className="art__ring art__ring--dashed" cx="150" cy="100" r="72" />
          <circle cx="150" cy="20" r="5" className="art__mark" />
          <circle cx="150" cy="180" r="3" className="art__tick" />
          <circle cx="70" cy="100" r="3" className="art__tick" />
          <circle cx="230" cy="100" r="3" className="art__tick" />
          <g className="art__wobble">
            <g transform="rotate(58 150 100)">
              <use href={`#${id}`} clipPath={`url(#${id}-clip)`} />
            </g>
          </g>
          <circle className="art__rim" cx="150" cy="100" r="62" />
        </svg>
        <span className="art__pill">
          当前偏移 <strong>+58°</strong>
        </span>
      </div>
      <Track progress={36} text="向右拖动滑块使图片摆正" />
    </div>
  );
}

function Track({ progress, text }: { progress: number; text: string }) {
  return (
    <div className="art__track" style={{ '--progress': `${progress}%` } as CSSProperties}>
      <span className="art__track-fill" />
      <span className="art__handle art__nudge">
        <svg
          viewBox="0 0 16 16"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          aria-hidden="true"
        >
          <path d="m4 4 4 4-4 4M9 4l4 4-4 4" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </span>
      <span className="art__track-text">{text}</span>
    </div>
  );
}

const TILES: ReadonlyArray<Tile> = [
  {
    id: 'click',
    title: '点选验证',
    mode: '依序点选',
    desc: '按提示顺序点击图中的文字或图形。',
    art: <ClickArt />,
    tries: [
      { type: 'click-text', label: '文字点选' },
      { type: 'click-shape', label: '图形点选' },
    ],
  },
  {
    id: 'slide',
    title: '滑块拼图',
    mode: '平移嵌合',
    desc: '拖动滑块，让拼图块与缺口吻合。',
    art: <SlideArt />,
    tries: [{ type: 'slide-text', label: '立即体验' }],
  },
  {
    id: 'region',
    title: '空间拼图',
    mode: '二维拖拽',
    desc: '把图块自由拖进缺口，松手即提交。',
    art: <RegionArt />,
    tries: [{ type: 'slide-region', label: '立即体验' }],
  },
  {
    id: 'rotate',
    title: '旋转验证',
    mode: '角度纠偏',
    desc: '拖动滑块，把圆形图片转回正向。',
    art: <RotateArt />,
    tries: [{ type: 'rotate', label: '立即体验' }],
  },
];
