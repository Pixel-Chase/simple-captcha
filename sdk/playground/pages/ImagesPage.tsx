import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ChangeEvent, DragEvent } from 'react';
import { adminApi, formatBytes, formatDateTime, getErrorMessage, isAdminApiError } from '../api';
import type { BackgroundItem, UploadConstraints } from '../api';
import { Icon } from '../components/Icon';
import type { IconName } from '../components/Icon';
import { Modal } from '../components/Modal';
import { useToast } from '../components/Toast';
import './ImagesPage.css';

interface ImagesPageProps {
  token: string;
  onSessionExpired: () => void;
}

interface UploadTask {
  id: number;
  /** 保留原始文件，失败后可直接重试 */
  file: File;
  status: 'uploading' | 'done' | 'error';
  /** 上传成功后服务端保存的文件名 */
  savedName?: string;
  /** 失败原因 */
  message?: string;
  /** 客户端预检不通过的文件没有发出请求，重试也不会通过，不提供重试 */
  retryable?: boolean;
}

/** 清晰度分档：高清（≥ 建议尺寸）、达标（≥ 最小尺寸）、低于下限 */
type Tier = 'hd' | 'ok' | 'low';
type Filter = 'all' | Tier;
type SortKey = 'newest' | 'oldest' | 'name';

type SizeSpec = Pick<
  UploadConstraints,
  'minWidth' | 'minHeight' | 'recommendedWidth' | 'recommendedHeight'
>;

/** 列表接口返回前的兜底规格，与 backend/services/backgrounds.go 的常量一致 */
const FALLBACK_SPEC: SizeSpec = {
  minWidth: 900,
  minHeight: 600,
  recommendedWidth: 1800,
  recommendedHeight: 1200,
};
const FALLBACK_FORMATS = ['jpeg', 'png'];
/** 格式名对应的扩展名：浏览器拿不到 MIME 类型时按扩展名判断 */
const FORMAT_EXTENSIONS: Record<string, string[]> = { jpeg: ['jpg', 'jpeg'], png: ['png'] };

const MAX_TASKS = 6;
const SKELETON_KEYS = ['s1', 's2', 's3', 's4'];
const LAST_ONE_HINT = '至少保留 1 张背景图，最后一张不能删除';

const TIERS: Record<Tier, { label: string; badge: string; tone: 'good' | 'primary' | 'warn' }> = {
  hd: { label: '高清', badge: '高清素材', tone: 'good' },
  ok: { label: '达标', badge: '达标', tone: 'primary' },
  low: { label: '低于下限', badge: '低于下限 · 出图会放大', tone: 'warn' },
};

const FILTERS: ReadonlyArray<{ id: Filter; label: string }> = [
  { id: 'all', label: '全部' },
  { id: 'hd', label: '高清' },
  { id: 'ok', label: '达标' },
  { id: 'low', label: '低于下限' },
];

const byName = (a: BackgroundItem, b: BackgroundItem) =>
  a.name.localeCompare(b.name, 'zh-CN', { numeric: true });
const byTime = (a: BackgroundItem, b: BackgroundItem) =>
  Date.parse(a.modifiedAt) - Date.parse(b.modifiedAt);

const SORTS: ReadonlyArray<{
  id: SortKey;
  label: string;
  compare: (a: BackgroundItem, b: BackgroundItem) => number;
}> = [
  { id: 'newest', label: '最新修改', compare: (a, b) => byTime(b, a) || byName(a, b) },
  { id: 'oldest', label: '最早修改', compare: (a, b) => byTime(a, b) || byName(a, b) },
  { id: 'name', label: '文件名', compare: byName },
];

function tierOf(item: BackgroundItem, spec: SizeSpec): Tier {
  if (item.width >= spec.recommendedWidth && item.height >= spec.recommendedHeight) {
    return 'hd';
  }
  if (item.width >= spec.minWidth && item.height >= spec.minHeight) {
    return 'ok';
  }
  return 'low';
}

/** 预览弹窗里对当前清晰度档位的说明 */
function tierNote(tier: Tier, spec: SizeSpec): string {
  const min = `${spec.minWidth}×${spec.minHeight}`;
  const rec = `${spec.recommendedWidth}×${spec.recommendedHeight}`;
  if (tier === 'hd') {
    return `达到建议尺寸 ${rec}，随机放大取景后依然清晰。`;
  }
  if (tier === 'ok') {
    return `达到最小尺寸 ${min}，3 倍率出图无需放大；换成 3:2、≥ ${rec} 的素材会更清晰。`;
  }
  return `低于最小尺寸 ${min}，3 倍率出图时会被放大而发糊，建议替换为 3:2、≥ ${rec} 的素材。`;
}

function isAllowedFile(file: File, formats: string[]): boolean {
  if (file.type) {
    return formats.some((format) => file.type === `image/${format}`);
  }
  const ext = file.name.split('.').pop()?.toLowerCase() ?? '';
  return formats.some((format) => (FORMAT_EXTENSIONS[format] ?? [format]).includes(ext));
}

/** 客户端预检：格式与体积不合格的文件直接标记，不发请求；尺寸与像素总数由服务端校验 */
function precheck(
  file: File,
  constraints: UploadConstraints | null,
  formats: string[],
): string | null {
  if (!isAllowedFile(file, formats)) {
    return `仅支持 ${formats.map((format) => format.toUpperCase()).join(' / ')}`;
  }
  if (constraints && file.size > constraints.maxUploadSize) {
    return `超过 ${formatBytes(constraints.maxUploadSize)} 上限`;
  }
  return null;
}

function formatPixels(pixels: number): string {
  return pixels >= 10_000 ? `${Math.round(pixels / 10_000)} 万` : String(pixels);
}

export function ImagesPage({ token, onSessionExpired }: ImagesPageProps) {
  const toast = useToast();
  const [items, setItems] = useState<BackgroundItem[]>([]);
  const [constraints, setConstraints] = useState<UploadConstraints | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [previews, setPreviews] = useState<Record<string, string>>({});
  const [brokenPreviews, setBrokenPreviews] = useState<ReadonlySet<string>>(() => new Set());
  const [tasks, setTasks] = useState<UploadTask[]>([]);
  const [dragging, setDragging] = useState(false);
  const [filter, setFilter] = useState<Filter>('all');
  const [sort, setSort] = useState<SortKey>('newest');
  const [previewName, setPreviewName] = useState<string | null>(null);
  const [confirmName, setConfirmName] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const inputRef = useRef<HTMLInputElement>(null);
  const previewsRef = useRef<Record<string, string>>({});
  const pendingPreviews = useRef(new Set<string>());
  const aliveRef = useRef(true);
  const taskSeq = useRef(0);

  /** 401 统一回到登录页，其余错误转成文案交给调用方展示 */
  const handleApiError = useCallback(
    (err: unknown, fallback: string): string | null => {
      if (isAdminApiError(err) && err.status === 401) {
        onSessionExpired();
        return null;
      }
      return getErrorMessage(err, fallback);
    },
    [onSessionExpired],
  );

  /** 预览接口需要携带令牌，取回 blob 生成对象 URL；同一张图同时只取一次 */
  const ensurePreview = useCallback(
    async (name: string) => {
      if (previewsRef.current[name] || pendingPreviews.current.has(name)) {
        return;
      }
      pendingPreviews.current.add(name);
      setBrokenPreviews((prev) => {
        if (!prev.has(name)) {
          return prev;
        }
        const next = new Set(prev);
        next.delete(name);
        return next;
      });
      try {
        const url = await adminApi.fetchPreview(token, name);
        if (!aliveRef.current) {
          URL.revokeObjectURL(url);
          return;
        }
        previewsRef.current = { ...previewsRef.current, [name]: url };
        setPreviews(previewsRef.current);
      } catch {
        // 单张预览失败不阻塞列表，卡片显示占位，下次刷新列表时重试
        setBrokenPreviews((prev) => new Set(prev).add(name));
      } finally {
        pendingPreviews.current.delete(name);
      }
    },
    [token],
  );

  const releasePreview = useCallback((name: string) => {
    const url = previewsRef.current[name];
    if (!url) {
      return;
    }
    URL.revokeObjectURL(url);
    const rest = Object.fromEntries(
      Object.entries(previewsRef.current).filter(([key]) => key !== name),
    );
    previewsRef.current = rest;
    setPreviews(rest);
  }, []);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await adminApi.listBackgrounds(token);
      setItems(data.items);
      setConstraints(data.constraints);
      // 已不在列表里的素材（例如在别的窗口删除）释放预览
      const names = new Set(data.items.map((item) => item.name));
      for (const name of Object.keys(previewsRef.current)) {
        if (!names.has(name)) {
          releasePreview(name);
        }
      }
      void Promise.all(data.items.map((item) => ensurePreview(item.name)));
    } catch (err) {
      const message = handleApiError(err, '加载背景图失败');
      if (message) {
        setError(message);
      }
    } finally {
      setLoading(false);
    }
  }, [token, ensurePreview, releasePreview, handleApiError]);

  useEffect(() => {
    void load();
  }, [load]);

  // 卸载时释放全部对象 URL；之后才返回的预览请求在 ensurePreview 里直接释放
  useEffect(() => {
    aliveRef.current = true;
    return () => {
      aliveRef.current = false;
      for (const url of Object.values(previewsRef.current)) {
        URL.revokeObjectURL(url);
      }
      previewsRef.current = {};
    };
  }, []);

  const patchTask = useCallback((id: number, patch: Partial<UploadTask>) => {
    setTasks((prev) => prev.map((task) => (task.id === id ? { ...task, ...patch } : task)));
  }, []);

  /** 上传单个文件并更新对应任务；会话失效时返回 false，调用方停止后续上传 */
  const runUpload = useCallback(
    async (id: number, file: File): Promise<boolean> => {
      patchTask(id, { status: 'uploading', message: undefined });
      try {
        const saved = await adminApi.uploadBackground(token, file);
        setItems((prev) => [saved, ...prev.filter((item) => item.name !== saved.name)]);
        patchTask(id, { status: 'done', savedName: saved.name });
        // 同名文件被覆盖时旧预览已过期，重新获取
        releasePreview(saved.name);
        void ensurePreview(saved.name);
        return true;
      } catch (err) {
        const message = handleApiError(err, '上传失败');
        if (!message) {
          return false;
        }
        patchTask(id, { status: 'error', message, retryable: true });
        return true;
      }
    },
    [token, patchTask, releasePreview, ensurePreview, handleApiError],
  );

  const formats = constraints?.allowedFormats ?? FALLBACK_FORMATS;
  const spec: SizeSpec = constraints ?? FALLBACK_SPEC;

  /** 多选时逐张上传，任务列表新的在上 */
  const uploadFiles = async (files: File[]) => {
    for (const file of files) {
      taskSeq.current += 1;
      const id = taskSeq.current;
      const reason = precheck(file, constraints, formats);
      const task: UploadTask = reason
        ? { id, file, status: 'error', message: reason, retryable: false }
        : { id, file, status: 'uploading' };
      setTasks((prev) => [task, ...prev].slice(0, MAX_TASKS));
      if (reason) {
        continue;
      }
      if (!(await runUpload(id, file))) {
        return;
      }
    }
  };

  const openFilePicker = () => inputRef.current?.click();

  const handleInputChange = (event: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? []);
    event.target.value = '';
    void uploadFiles(files);
  };

  const isFileDrag = (event: DragEvent<HTMLElement>) =>
    Array.from(event.dataTransfer.types).includes('Files');

  const handleDragOver = (event: DragEvent<HTMLElement>) => {
    if (!isFileDrag(event)) {
      return;
    }
    event.preventDefault();
    event.dataTransfer.dropEffect = 'copy';
    setDragging(true);
  };

  const handleDragLeave = (event: DragEvent<HTMLElement>) => {
    // 在子元素之间移动也会触发 dragleave，只有真正离开拖放区才取消高亮
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
      setDragging(false);
    }
  };

  const handleDrop = (event: DragEvent<HTMLElement>) => {
    event.preventDefault();
    setDragging(false);
    void uploadFiles(Array.from(event.dataTransfer.files));
  };

  const openDelete = (name: string) => {
    setDeleteError(null);
    setPreviewName(null);
    setConfirmName(name);
  };

  const closeDelete = () => {
    if (!deleting) {
      setConfirmName(null);
    }
  };

  const handleDelete = async (name: string) => {
    setDeleting(true);
    setDeleteError(null);
    try {
      await adminApi.deleteBackground(token, name);
      setItems((prev) => prev.filter((item) => item.name !== name));
      releasePreview(name);
      setConfirmName(null);
      toast(`已删除 ${name}`);
    } catch (err) {
      const message = handleApiError(err, '删除失败');
      if (message) {
        setDeleteError(message);
      }
    } finally {
      setDeleting(false);
    }
  };

  const tiers = useMemo(
    () => new Map(items.map((item) => [item.name, tierOf(item, spec)])),
    [items, spec],
  );

  const counts = useMemo(() => {
    const result: Record<Filter, number> = { all: items.length, hd: 0, ok: 0, low: 0 };
    for (const tier of tiers.values()) {
      result[tier] += 1;
    }
    return result;
  }, [items.length, tiers]);

  const visibleItems = useMemo(() => {
    const compare = SORTS.find((option) => option.id === sort)?.compare ?? SORTS[0].compare;
    const filtered =
      filter === 'all' ? items : items.filter((item) => tiers.get(item.name) === filter);
    return [...filtered].sort(compare);
  }, [items, tiers, filter, sort]);

  const ready = constraints !== null;
  const onlyOneLeft = items.length <= 1;
  const hasFinishedTasks = tasks.some((task) => task.status !== 'uploading');
  const accept = formats
    .flatMap((format) => [
      `image/${format}`,
      ...(FORMAT_EXTENSIONS[format] ?? [format]).map((ext) => `.${ext}`),
    ])
    .join(',');
  const previewItem = previewName ? items.find((item) => item.name === previewName) : undefined;
  const confirmItem = confirmName ? items.find((item) => item.name === confirmName) : undefined;

  const renderThumb = (item: BackgroundItem, imgClass: string, alt: string) => {
    const url = previews[item.name];
    if (url) {
      return <img className={imgClass} src={url} alt={alt} decoding="async" />;
    }
    if (brokenPreviews.has(item.name)) {
      return (
        <span className="images-card__placeholder">
          <Icon name="image" size={24} />
          预览加载失败
        </span>
      );
    }
    return <span className="images__shimmer images-card__skeleton" />;
  };

  const renderLibrary = () => {
    if (!ready && loading) {
      return (
        <ul className="images__grid" aria-busy="true" aria-label="正在加载素材">
          {SKELETON_KEYS.map((key) => (
            <li key={key} className="images-card">
              <span className="images__shimmer images-card__media images-card__media--static" />
              <div className="images-card__body">
                <span className="images__shimmer images-card__line" />
                <span className="images__shimmer images-card__line images-card__line--short" />
              </div>
            </li>
          ))}
        </ul>
      );
    }

    if (!ready && error) {
      return (
        <div className="images__state images__state--error">
          <span className="images__state-icon">
            <Icon name="error" size={28} />
          </span>
          <p className="images__state-title">素材列表加载失败</p>
          <p className="images__state-desc">{error}</p>
          <button
            type="button"
            className="btn btn--primary images__state-action"
            onClick={() => void load()}
          >
            <Icon name="refresh" size={16} />
            重新加载
          </button>
        </div>
      );
    }

    if (items.length === 0) {
      return (
        <div className="images__state">
          <span className="images__state-icon">
            <Icon name="image" size={28} />
          </span>
          <p className="images__state-title">素材库还是空的</p>
          <p className="images__state-desc">
            上传至少一张 {formats.map((format) => format.toUpperCase()).join(' / ')}{' '}
            背景图，所有题型即可用它出题。
          </p>
          <button
            type="button"
            className="btn btn--primary images__state-action"
            onClick={openFilePicker}
          >
            <Icon name="upload" size={16} />
            上传素材
          </button>
        </div>
      );
    }

    if (visibleItems.length === 0) {
      const label = FILTERS.find((option) => option.id === filter)?.label ?? '';
      return (
        <div className="images__state">
          <span className="images__state-icon">
            <Icon name="filter" size={28} />
          </span>
          <p className="images__state-title">没有「{label}」档的素材</p>
          <p className="images__state-desc">换一个分类看看，或查看全部素材。</p>
          <button
            type="button"
            className="btn images__state-action"
            onClick={() => setFilter('all')}
          >
            查看全部
          </button>
        </div>
      );
    }

    return (
      <ul className="images__grid">
        {visibleItems.map((item) => {
          const tier = tiers.get(item.name) ?? 'ok';
          const meta = TIERS[tier];
          return (
            <li
              key={item.name}
              className={`images-card${tier === 'low' ? ' images-card--low' : ''}`}
            >
              <button
                type="button"
                className="images-card__media"
                onClick={() => setPreviewName(item.name)}
                aria-label={`预览 ${item.name}`}
              >
                {renderThumb(item, 'images-card__img', '')}
                <span className={`chip chip--${meta.tone} images-card__badge`}>
                  {tier === 'low' ? (
                    <Icon name="warning" size={12} />
                  ) : (
                    <span className="chip__dot" />
                  )}
                  {meta.badge}
                </span>
                <span className="images-card__zoom">
                  <Icon name="zoom-in" size={16} />
                </span>
              </button>
              <div className="images-card__body">
                <div>
                  <div className="images-card__title">
                    <span className="images-card__name" title={item.name}>
                      {item.name}
                    </span>
                    <span className="images-card__format">{item.format.toUpperCase()}</span>
                  </div>
                  <div className="images-card__meta">
                    <span className={tier === 'low' ? 'images-card__meta-warn' : undefined}>
                      尺寸：{item.width} × {item.height}
                    </span>
                    <span>大小：{formatBytes(item.size)}</span>
                    <span className="images-card__meta-wide">
                      更新：{formatDateTime(item.modifiedAt)}
                    </span>
                  </div>
                </div>
                <div className="images-card__actions">
                  <button
                    type="button"
                    className="images-card__action"
                    onClick={() => setPreviewName(item.name)}
                  >
                    <Icon name="eye" size={16} />
                    预览
                  </button>
                  <button
                    type="button"
                    className="images-card__action images-card__action--danger"
                    onClick={() => openDelete(item.name)}
                    disabled={onlyOneLeft}
                    title={onlyOneLeft ? LAST_ONE_HINT : undefined}
                  >
                    <Icon name={onlyOneLeft ? 'lock' : 'trash'} size={16} />
                    删除
                  </button>
                </div>
              </div>
            </li>
          );
        })}
      </ul>
    );
  };

  return (
    <div className="page images">
      <input
        ref={inputRef}
        type="file"
        accept={accept}
        multiple
        hidden
        onChange={handleInputChange}
      />

      <header className="page-head images__head">
        <div className="images__intro">
          <div className="page-head__title">
            <h1 className="page-title">背景图素材管理</h1>
            <span className="chip chip--primary">全题型通用</span>
          </div>
          <p className="page-desc">
            上传或删除后立即生效，所有题型下一次出题即使用新素材，无需重启服务。
          </p>
        </div>
        <div className="page-head__actions images__head-actions">
          <div className="images__summary">
            <div>
              <div className="images__summary-label">素材库总览</div>
              <div className="images__summary-total">
                {ready ? `共 ${items.length} 张` : loading ? '加载中…' : '暂无数据'}
              </div>
            </div>
            {ready && (
              <>
                <span className="images__summary-sep" />
                <div className="images__summary-tiers">
                  <span className="chip chip--good">{counts.hd} 张高清</span>
                  <span className="chip chip--primary">{counts.ok} 张达标</span>
                  <span className={counts.low > 0 ? 'chip chip--warn' : 'chip'}>
                    {counts.low} 张低于下限
                  </span>
                </div>
              </>
            )}
          </div>
          <button
            type="button"
            className="btn btn--primary images__upload-btn"
            onClick={openFilePicker}
          >
            <Icon name="upload" size={18} />
            上传素材
          </button>
        </div>
      </header>

      <div
        className={`images__notice${ready && onlyOneLeft ? ' images__notice--warn' : ''}`}
        role="note"
      >
        <Icon name="shield-lock" size={20} />
        <p>
          <strong className="images__notice-title">出题保底：</strong>
          至少保留 1 张背景图，只剩 1 张时不能删除，避免出题时没有可用素材。
          {ready && onlyOneLeft && '当前只剩 1 张，删除已锁定；需要更换时请先上传新素材。'}
        </p>
      </div>

      <div className="images__workbench">
        <div className="images__upload">
          <button
            type="button"
            className={`images__dropzone${dragging ? ' images__dropzone--active' : ''}`}
            onClick={openFilePicker}
            onDragEnter={handleDragOver}
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
          >
            <span className="images__drop-icon">
              <Icon name={dragging ? 'upload' : 'image'} size={28} />
            </span>
            <span className="images__drop-title">
              {dragging ? '松开即可上传' : '拖拽背景图到这里，或点击选择文件'}
            </span>
            <span className="images__drop-desc">
              可一次选择多张，逐张上传并校验，保存后立即用于出题
            </span>
            <span className="images__drop-specs">
              {constraints && (
                <span className="chip">单张 ≤ {formatBytes(constraints.maxUploadSize)}</span>
              )}
              <span className="chip">
                {formats.map((format) => format.toUpperCase()).join(' / ')}
              </span>
              <span className="chip">
                最小 {spec.minWidth}×{spec.minHeight}
              </span>
              <span className="chip">建议 3:2 比例</span>
            </span>
          </button>

          {tasks.length > 0 && (
            <section className="images__tasks" aria-label="最近上传">
              <div className="images__tasks-head">
                <span>最近上传</span>
                {hasFinishedTasks && (
                  <button
                    type="button"
                    className="btn btn--text images__tasks-clear"
                    onClick={() =>
                      setTasks((prev) => prev.filter((task) => task.status === 'uploading'))
                    }
                  >
                    清空记录
                  </button>
                )}
              </div>
              <ul className="images__tasks-list">
                {tasks.map((task) => (
                  <li key={task.id} className={`images-task images-task--${task.status}`}>
                    <span className="images-task__icon">
                      {task.status === 'uploading' && (
                        <Icon name="spinner" size={18} className="spin" />
                      )}
                      {task.status === 'done' && <Icon name="check-circle" size={18} />}
                      {task.status === 'error' && <Icon name="error" size={18} />}
                    </span>
                    <span className="images-task__text">
                      <span className="images-task__name" title={task.file.name}>
                        {task.file.name}
                      </span>
                      <span className="images-task__message">
                        {task.status === 'uploading' && '上传中…'}
                        {task.status === 'done' && `→ 已保存为 ${task.savedName}`}
                        {task.status === 'error' && task.message}
                      </span>
                    </span>
                    <span className="images-task__side">
                      {task.status === 'uploading' && (
                        <span className="chip chip--primary">上传中</span>
                      )}
                      {task.status === 'done' && <span className="chip chip--good">已保存</span>}
                      {task.status === 'error' && (
                        <span className="chip chip--danger">
                          {task.retryable ? '失败' : '未上传'}
                        </span>
                      )}
                      {task.status === 'error' && task.retryable && (
                        <button
                          type="button"
                          className="btn btn--text images-task__retry"
                          onClick={() => void runUpload(task.id, task.file)}
                        >
                          重试
                        </button>
                      )}
                    </span>
                  </li>
                ))}
              </ul>
            </section>
          )}
        </div>

        <aside className="images__spec" aria-label="素材规格">
          <div>
            <div className="images__spec-eyebrow">
              <span>素材规格</span>
              <Icon name="sliders" size={20} />
            </div>
            <h2 className="images__spec-title">为什么分辨率重要？</h2>
            <p className="images__spec-lead">
              题目图片按访客屏幕的设备像素比出图，最高 3 倍（{spec.minWidth}×{spec.minHeight}
              ），并在此基础上随机放大取景。素材分辨率不足时只能被拉伸放大，题目图片随之变糊。
            </p>
            <ul className="images__spec-tiers">
              <li className="images__spec-tier images__spec-tier--hd">
                <Icon name="shield-check" size={18} />
                <p>
                  <strong className="images__spec-tier-name">高清：</strong>≥{' '}
                  {spec.recommendedWidth}×{spec.recommendedHeight}，宽高比
                  3:2，为随机放大取景留足余量。
                </p>
              </li>
              <li className="images__spec-tier images__spec-tier--ok">
                <Icon name="check" size={18} />
                <p>
                  <strong className="images__spec-tier-name">达标：</strong>≥ {spec.minWidth}×
                  {spec.minHeight}，3 倍率出图无需放大，可以正常使用。
                </p>
              </li>
              <li className="images__spec-tier images__spec-tier--low">
                <Icon name="warning" size={18} />
                <p>
                  <strong className="images__spec-tier-name">低于下限：</strong>&lt; {spec.minWidth}
                  ×{spec.minHeight}，3
                  倍率出图时会被放大而发糊。上传会被拒绝，只会出现在直接放进素材目录的文件里。
                </p>
              </li>
            </ul>
          </div>
          {constraints && (
            <p className="images__spec-foot">
              上传时依次校验体积、格式、尺寸下限与像素总数（≤ {formatPixels(constraints.maxPixels)}
              ），不合格的文件不会保存。
            </p>
          )}
        </aside>
      </div>

      <section className="images__library" aria-label="当前素材库">
        <div className="images__toolbar">
          <div className="images__toolbar-start">
            <h2 className="images__library-title">当前素材库 ({items.length})</h2>
            <span className="images__toolbar-sep" />
            <fieldset className="images__filter" aria-label="按清晰度筛选">
              {FILTERS.map((option) => (
                <button
                  key={option.id}
                  type="button"
                  className={`images__filter-btn${filter === option.id ? ' images__filter-btn--active' : ''}`}
                  aria-pressed={filter === option.id}
                  onClick={() => setFilter(option.id)}
                >
                  {option.label} ({counts[option.id]})
                </button>
              ))}
            </fieldset>
          </div>
          <div className="images__toolbar-end">
            <button
              type="button"
              className="btn btn--sm images__refresh"
              onClick={() => void load()}
              disabled={loading}
            >
              <Icon name="sync" size={16} className={loading ? 'spin' : undefined} />
              {loading ? '刷新中…' : '刷新列表'}
            </button>
            <label className="images__sort">
              <span>排序：</span>
              <select
                className="images__sort-select"
                value={sort}
                onChange={(event) => setSort(event.target.value as SortKey)}
              >
                {SORTS.map((option) => (
                  <option key={option.id} value={option.id}>
                    {option.label}
                  </option>
                ))}
              </select>
              <Icon name="chevron-down" size={16} className="images__sort-icon" />
            </label>
          </div>
        </div>

        {ready && error && (
          <div className="alert alert--error" role="alert">
            <Icon name="error" size={16} />
            <span>刷新失败：{error}</span>
          </div>
        )}

        {renderLibrary()}
      </section>

      {previewItem && (
        <Modal
          open
          onClose={() => setPreviewName(null)}
          size="lg"
          icon="image"
          title={<span className="images-preview__title">{previewItem.name}</span>}
          subtitle={`${previewItem.width} × ${previewItem.height} · ${previewItem.format.toUpperCase()} · ${formatBytes(previewItem.size)}`}
          footer={
            <>
              <button
                type="button"
                className="btn btn--text btn--danger-text images-preview__delete"
                onClick={() => openDelete(previewItem.name)}
                disabled={onlyOneLeft}
                title={onlyOneLeft ? LAST_ONE_HINT : undefined}
              >
                <Icon name="trash" size={16} />
                删除
              </button>
              <button type="button" className="btn" onClick={() => setPreviewName(null)}>
                关闭
              </button>
            </>
          }
        >
          <div className="images-preview__frame">
            {renderThumb(previewItem, 'images-preview__img', `背景图 ${previewItem.name}`)}
          </div>
          <PreviewInfo item={previewItem} tier={tiers.get(previewItem.name) ?? 'ok'} spec={spec} />
        </Modal>
      )}

      {confirmItem && (
        <Modal
          open
          onClose={closeDelete}
          size="sm"
          icon="trash"
          tone="danger"
          title="确认删除此素材？"
          dismissible={!deleting}
          footer={
            <>
              <button type="button" className="btn" onClick={closeDelete} disabled={deleting}>
                取消
              </button>
              <button
                type="button"
                className="btn btn--danger"
                onClick={() => void handleDelete(confirmItem.name)}
                disabled={deleting || onlyOneLeft}
              >
                {deleting && <Icon name="spinner" size={16} className="spin" />}
                {deleting ? '删除中…' : '确认删除'}
              </button>
            </>
          }
        >
          <p className="images-delete__desc">
            删除后不可恢复，所有题型下一次出题将不再使用这张图片：
          </p>
          <p className="images-delete__target">{confirmItem.name}</p>
          {onlyOneLeft ? (
            <div className="alert alert--warn images-delete__note">
              <Icon name="warning" size={16} />
              <span>{LAST_ONE_HINT}。</span>
            </div>
          ) : (
            <p className="images-delete__hint">
              <Icon name="info" size={16} />
              删除后素材库剩余 {items.length - 1} 张。
            </p>
          )}
          {deleteError && (
            <div className="alert alert--error images-delete__note" role="alert">
              <Icon name="error" size={16} />
              <span>{deleteError}</span>
            </div>
          )}
        </Modal>
      )}
    </div>
  );
}

const NOTE_STYLE: Record<Tier, { className: string; icon: IconName }> = {
  hd: { className: 'alert--success', icon: 'check-circle' },
  ok: { className: '', icon: 'info' },
  low: { className: 'alert--warn', icon: 'warning' },
};

/** 预览弹窗下方的文件信息与清晰度说明 */
function PreviewInfo({ item, tier, spec }: { item: BackgroundItem; tier: Tier; spec: SizeSpec }) {
  const meta = TIERS[tier];
  return (
    <>
      <dl className="images-preview__info">
        <div>
          <dt className="images-preview__label">清晰度</dt>
          <dd className="images-preview__value">
            <span className={`chip chip--${meta.tone}`}>{meta.label}</span>
          </dd>
        </div>
        <div>
          <dt className="images-preview__label">尺寸</dt>
          <dd className="images-preview__value">
            {item.width} × {item.height}
          </dd>
        </div>
        <div>
          <dt className="images-preview__label">格式</dt>
          <dd className="images-preview__value">{item.format.toUpperCase()}</dd>
        </div>
        <div>
          <dt className="images-preview__label">大小</dt>
          <dd className="images-preview__value">{formatBytes(item.size)}</dd>
        </div>
        <div>
          <dt className="images-preview__label">更新时间</dt>
          <dd className="images-preview__value">{formatDateTime(item.modifiedAt)}</dd>
        </div>
      </dl>
      <div className={`alert ${NOTE_STYLE[tier].className} images-preview__note`}>
        <Icon name={NOTE_STYLE[tier].icon} size={16} />
        <span>{tierNote(tier, spec)}</span>
      </div>
    </>
  );
}
