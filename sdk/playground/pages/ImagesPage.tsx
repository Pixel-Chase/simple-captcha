import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ChangeEvent, DragEvent } from 'react';
import { adminApi, formatBytes, formatDateTime, getErrorMessage, isAdminApiError } from '../api';
import type { BackgroundItem, UploadConstraints } from '../api';

interface ImagesPageProps {
  token: string;
  onSessionExpired: () => void;
}

interface UploadTask {
  id: number;
  filename: string;
  status: 'uploading' | 'done' | 'error';
  message?: string;
}

const ACCEPTED_TYPES = ['image/png', 'image/jpeg'];
const MAX_VISIBLE_TASKS = 6;

/** 按基础出图尺寸换算素材能支撑的最高清晰度 */
function qualityOf(item: BackgroundItem, constraints: UploadConstraints | null) {
  const minW = constraints?.minWidth ?? 300;
  const minH = constraints?.minHeight ?? 220;
  if (item.width >= minW * 3 && item.height >= minH * 3) {
    return { text: '3x 高清', tone: 'good' as const };
  }
  if (item.width >= minW * 2 && item.height >= minH * 2) {
    return { text: '2x', tone: 'ok' as const };
  }
  return { text: '1x · 高倍率会放大', tone: 'warn' as const };
}

/** 客户端预检：类型与体积不合格的文件直接标记，不发请求 */
function precheck(file: File, constraints: UploadConstraints | null): string | null {
  if (!ACCEPTED_TYPES.includes(file.type)) {
    return '仅支持 JPEG / PNG';
  }
  if (constraints && file.size > constraints.maxUploadSize) {
    return `超过 ${formatBytes(constraints.maxUploadSize)} 上限`;
  }
  return null;
}

export function ImagesPage({ token, onSessionExpired }: ImagesPageProps) {
  const [items, setItems] = useState<BackgroundItem[]>([]);
  const [constraints, setConstraints] = useState<UploadConstraints | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [previews, setPreviews] = useState<Record<string, string>>({});
  const [tasks, setTasks] = useState<UploadTask[]>([]);
  const [confirmName, setConfirmName] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [dragging, setDragging] = useState(false);

  const inputRef = useRef<HTMLInputElement>(null);
  const previewsRef = useRef<Record<string, string>>({});
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

  const ensurePreview = useCallback(
    async (name: string) => {
      if (previewsRef.current[name]) {
        return;
      }
      try {
        const url = await adminApi.fetchPreview(token, name);
        previewsRef.current = { ...previewsRef.current, [name]: url };
        setPreviews(previewsRef.current);
      } catch {
        // 单张预览失败不阻塞列表，卡片保持占位状态
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
      void Promise.all(data.items.map((item) => ensurePreview(item.name)));
    } catch (err) {
      const message = handleApiError(err, '加载背景图失败');
      if (message) {
        setError(message);
      }
    } finally {
      setLoading(false);
    }
  }, [token, ensurePreview, handleApiError]);

  useEffect(() => {
    void load();
  }, [load]);

  // 卸载时释放全部对象 URL
  useEffect(
    () => () => {
      for (const url of Object.values(previewsRef.current)) {
        URL.revokeObjectURL(url);
      }
    },
    [],
  );

  const uploadFiles = async (files: File[]) => {
    for (const file of files) {
      taskSeq.current += 1;
      const id = taskSeq.current;

      const reason = precheck(file, constraints);
      if (reason) {
        setTasks((prev) => [
          { id, filename: file.name, status: 'error', message: reason },
          ...prev,
        ]);
        continue;
      }

      setTasks((prev) => [{ id, filename: file.name, status: 'uploading' }, ...prev]);
      try {
        const saved = await adminApi.uploadBackground(token, file);
        setItems((prev) => [saved, ...prev.filter((item) => item.name !== saved.name)]);
        setTasks((prev) =>
          prev.map((task) =>
            task.id === id ? { ...task, status: 'done', message: `已保存为 ${saved.name}` } : task,
          ),
        );
        void ensurePreview(saved.name);
      } catch (err) {
        const message = handleApiError(err, '上传失败');
        if (!message) {
          return;
        }
        setTasks((prev) =>
          prev.map((task) => (task.id === id ? { ...task, status: 'error', message } : task)),
        );
      }
    }
  };

  const handleInputChange = (event: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? []);
    event.target.value = '';
    void uploadFiles(files);
  };

  const handleDrop = (event: DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    setDragging(false);
    void uploadFiles(Array.from(event.dataTransfer.files));
  };

  const handleDelete = async (name: string) => {
    setDeleting(name);
    setError(null);
    try {
      await adminApi.deleteBackground(token, name);
      setItems((prev) => prev.filter((item) => item.name !== name));
      releasePreview(name);
      setConfirmName(null);
    } catch (err) {
      const message = handleApiError(err, '删除失败');
      if (message) {
        setError(message);
      }
    } finally {
      setDeleting(null);
    }
  };

  const hint = useMemo(() => {
    if (!constraints) {
      return '';
    }
    const formats = constraints.allowedFormats.map((f) => f.toUpperCase()).join(' / ');
    return [
      formats,
      `最小 ${constraints.minWidth}×${constraints.minHeight}`,
      `单张 ≤ ${formatBytes(constraints.maxUploadSize)}`,
      `3x 高清建议 ≥ ${constraints.minWidth * 3}×${constraints.minHeight * 3}`,
    ].join(' · ');
  }, [constraints]);

  const onlyOneLeft = items.length <= 1;

  return (
    <section className="images">
      <div className="images__head">
        <div>
          <p className="eyebrow">素材</p>
          <h1>背景图管理</h1>
          <p className="muted">
            共 {items.length}{' '}
            张。上传或删除后立即生效：所有题型下一次出题即使用新素材，无需重启服务。
          </p>
        </div>
        <button
          type="button"
          className="btn btn--ghost"
          onClick={() => void load()}
          disabled={loading}
        >
          {loading ? '刷新中…' : '刷新'}
        </button>
      </div>

      <div
        className={`dropzone${dragging ? ' dropzone--active' : ''}`}
        onDragOver={(event) => {
          event.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={handleDrop}
        onClick={() => inputRef.current?.click()}
      >
        <input
          ref={inputRef}
          type="file"
          accept={ACCEPTED_TYPES.join(',')}
          multiple
          hidden
          onChange={handleInputChange}
        />
        <div className="dropzone__title">拖拽图片到这里，或点击选择文件</div>
        <div className="dropzone__hint">{hint}</div>
      </div>

      {tasks.length > 0 && (
        <ul className="tasks">
          {tasks.slice(0, MAX_VISIBLE_TASKS).map((task) => (
            <li key={task.id} className={`task task--${task.status}`}>
              <span className="task__name" title={task.filename}>
                {task.filename}
              </span>
              <span className="task__message">
                {task.status === 'uploading' ? '上传中…' : task.message}
              </span>
            </li>
          ))}
        </ul>
      )}

      {error && (
        <div className="alert alert--error" role="alert">
          {error}
        </div>
      )}

      {loading && items.length === 0 ? (
        <div className="page-status">正在加载…</div>
      ) : (
        <ul className="grid">
          {items.map((item) => {
            const quality = qualityOf(item, constraints);
            const preview = previews[item.name];
            const confirming = confirmName === item.name;
            const removing = deleting === item.name;
            return (
              <li key={item.name} className="card tile">
                <div className="tile__preview">
                  {preview ? (
                    <img src={preview} alt={`背景图 ${item.name}`} />
                  ) : (
                    <div className="tile__skeleton" />
                  )}
                  <span className={`badge badge--${quality.tone}`}>{quality.text}</span>
                </div>
                <div className="tile__body">
                  <div className="tile__name" title={item.name}>
                    {item.name}
                  </div>
                  <div className="tile__meta">
                    {item.width}×{item.height} · {formatBytes(item.size)} ·{' '}
                    {item.format.toUpperCase()}
                  </div>
                  <div className="tile__meta muted">{formatDateTime(item.modifiedAt)}</div>
                </div>
                <div className="tile__actions">
                  {confirming ? (
                    <>
                      <span className="tile__confirm">确认删除？</span>
                      <button
                        type="button"
                        className="btn btn--danger btn--sm"
                        onClick={() => void handleDelete(item.name)}
                        disabled={removing}
                      >
                        {removing ? '删除中…' : '确认'}
                      </button>
                      <button
                        type="button"
                        className="btn btn--ghost btn--sm"
                        onClick={() => setConfirmName(null)}
                        disabled={removing}
                      >
                        取消
                      </button>
                    </>
                  ) : (
                    <button
                      type="button"
                      className="btn btn--ghost btn--sm"
                      onClick={() => setConfirmName(item.name)}
                      disabled={onlyOneLeft}
                      title={onlyOneLeft ? '至少保留一张背景图' : undefined}
                    >
                      删除
                    </button>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
