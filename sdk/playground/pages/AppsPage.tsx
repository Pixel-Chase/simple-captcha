import { useCallback, useEffect, useState } from 'react';
import type { FormEvent } from 'react';
import {
  API_BASE,
  API_PREFIX,
  adminApi,
  formatDateTime,
  getErrorMessage,
  isAdminApiError,
} from '../api';
import type { AppItem, AppSecretResponse } from '../api';

interface AppsPageProps {
  token: string;
  onSessionExpired: () => void;
}

/** 刚创建或刚重置的密钥：只在这里展示一次，用户确认保存后清除 */
interface RevealedSecret {
  name: string;
  secret: string;
  reason: 'created' | 'rotated';
}

type PendingAction = { id: string; action: 'rotate' | 'delete' };

const NAME_MAX_LENGTH = 40;

const stateCurl = (secret: string) =>
  `curl -H "Authorization: Bearer ${secret}" \\\n  ${API_BASE}${API_PREFIX}/captchas/states/<second_key>`;

export function AppsPage({ token, onSessionExpired }: AppsPageProps) {
  const [items, setItems] = useState<AppItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [newName, setNewName] = useState('');
  const [creating, setCreating] = useState(false);
  const [revealed, setRevealed] = useState<RevealedSecret | null>(null);
  const [copied, setCopied] = useState<'secret' | 'curl' | null>(null);
  const [pending, setPending] = useState<PendingAction | null>(null);
  const [editing, setEditing] = useState<{ id: string; name: string } | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

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

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await adminApi.listApps(token);
      setItems(data.items);
    } catch (err) {
      const message = handleApiError(err, '加载应用失败');
      if (message) {
        setError(message);
      }
    } finally {
      setLoading(false);
    }
  }, [token, handleApiError]);

  useEffect(() => {
    void load();
  }, [load]);

  const replaceItem = (app: AppItem) =>
    setItems((prev) => prev.map((item) => (item.id === app.id ? app : item)));

  const reveal = (result: AppSecretResponse, reason: RevealedSecret['reason']) => {
    setRevealed({ name: result.app.name, secret: result.secret, reason });
    setCopied(null);
  };

  /** 包一层统一的 busy / 错误处理，返回是否成功 */
  const run = async (id: string | null, fallback: string, task: () => Promise<void>) => {
    setBusyId(id);
    setError(null);
    try {
      await task();
      return true;
    } catch (err) {
      const message = handleApiError(err, fallback);
      if (message) {
        setError(message);
      }
      return false;
    } finally {
      setBusyId(null);
    }
  };

  const handleCreate = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const name = newName.trim();
    if (!name || creating) {
      return;
    }
    setCreating(true);
    const ok = await run(null, '创建应用失败', async () => {
      const result = await adminApi.createApp(token, name);
      setItems((prev) => [...prev, result.app]);
      reveal(result, 'created');
    });
    if (ok) {
      setNewName('');
    }
    setCreating(false);
  };

  const handleRename = async (app: AppItem) => {
    if (!editing) {
      return;
    }
    const name = editing.name.trim();
    if (!name || name === app.name) {
      setEditing(null);
      return;
    }
    const ok = await run(app.id, '重命名失败', async () => {
      replaceItem(await adminApi.updateApp(token, app.id, { name, enabled: app.enabled }));
    });
    if (ok) {
      setEditing(null);
    }
  };

  const handleToggle = (app: AppItem) =>
    run(app.id, app.enabled ? '停用失败' : '启用失败', async () => {
      replaceItem(
        await adminApi.updateApp(token, app.id, { name: app.name, enabled: !app.enabled }),
      );
    });

  const handleRotate = (app: AppItem) =>
    run(app.id, '重置密钥失败', async () => {
      const result = await adminApi.rotateAppSecret(token, app.id);
      replaceItem(result.app);
      reveal(result, 'rotated');
      setPending(null);
    });

  const handleDelete = (app: AppItem) =>
    run(app.id, '删除失败', async () => {
      await adminApi.deleteApp(token, app.id);
      setItems((prev) => prev.filter((item) => item.id !== app.id));
      setPending(null);
    });

  const copy = async (kind: 'secret' | 'curl', text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(kind);
    } catch {
      setCopied(null);
    }
  };

  return (
    <section className="apps">
      <div className="images__head">
        <div>
          <p className="eyebrow">S2S</p>
          <h1>接入应用</h1>
          <p className="muted">
            每个业务后端一个应用、一个独立密钥。业务后端携带{' '}
            <code>Authorization: Bearer &lt;应用密钥&gt;</code> 调用{' '}
            <code>GET {API_PREFIX}/captchas/states/:key</code>{' '}
            完成二次校验；停用、重置或删除后旧密钥立即失效。
          </p>
        </div>
        <button
          type="button"
          className="btn btn--ghost apps__refresh"
          onClick={() => void load()}
          disabled={loading}
        >
          {loading ? '刷新中…' : '刷新'}
        </button>
      </div>

      <form className="card apps__create" onSubmit={handleCreate}>
        <input
          className="field__input apps__create-input"
          placeholder="应用名称，例如：订单系统"
          value={newName}
          maxLength={NAME_MAX_LENGTH}
          onChange={(e) => setNewName(e.target.value)}
          disabled={creating}
          aria-label="应用名称"
        />
        <button type="submit" className="btn btn--primary" disabled={creating || !newName.trim()}>
          {creating ? '创建中…' : '创建应用'}
        </button>
      </form>

      {revealed && (
        <div className="card secret" role="status">
          <div className="secret__head">
            <span className="badge badge--warn">只显示这一次</span>
            <strong>
              {revealed.reason === 'created' ? '已创建' : '已重置'}「{revealed.name}」的密钥
            </strong>
          </div>
          <p className="muted secret__note">
            请立即复制并配置到业务后端。服务端只保存哈希，离开或关闭此提示后无法再次查看，遗失只能重置。
            {revealed.reason === 'rotated' && '旧密钥已经失效。'}
          </p>
          <code className="secret__value">{revealed.secret}</code>
          <pre className="demo__snippet">
            <code>{stateCurl(revealed.secret)}</code>
          </pre>
          <div className="secret__actions">
            <button
              type="button"
              className="btn btn--ghost btn--sm"
              onClick={() => void copy('secret', revealed.secret)}
            >
              {copied === 'secret' ? '已复制密钥' : '复制密钥'}
            </button>
            <button
              type="button"
              className="btn btn--ghost btn--sm"
              onClick={() => void copy('curl', stateCurl(revealed.secret))}
            >
              {copied === 'curl' ? '已复制命令' : '复制校验命令'}
            </button>
            <button
              type="button"
              className="btn btn--primary btn--sm secret__done"
              onClick={() => setRevealed(null)}
            >
              我已保存
            </button>
          </div>
        </div>
      )}

      {error && (
        <div className="alert alert--error" role="alert">
          {error}
        </div>
      )}

      {loading && items.length === 0 ? (
        <div className="page-status">正在加载…</div>
      ) : items.length === 0 ? (
        <div className="card apps__empty">
          <strong>还没有接入应用</strong>
          <p className="muted apps__empty-text">
            创建第一个应用之前，S2S 状态查询会一律返回
            401。为每个调用验证码服务的业务后端各建一个应用。
          </p>
        </div>
      ) : (
        <ul className="app-list">
          {items.map((app) => {
            const busy = busyId === app.id;
            const confirming = pending?.id === app.id ? pending.action : null;
            const renaming = editing?.id === app.id;
            return (
              <li
                key={app.id}
                className={`card app-card${app.enabled ? '' : ' app-card--disabled'}`}
              >
                <div className="app-card__main">
                  {renaming ? (
                    <form
                      className="app-card__rename"
                      onSubmit={(event) => {
                        event.preventDefault();
                        void handleRename(app);
                      }}
                    >
                      <input
                        className="field__input app-card__rename-input"
                        value={editing?.name ?? ''}
                        maxLength={NAME_MAX_LENGTH}
                        onChange={(e) => setEditing({ id: app.id, name: e.target.value })}
                        disabled={busy}
                        aria-label="新的应用名称"
                        // biome-ignore lint/a11y/noAutofocus: 点击「重命名」后直接进入编辑
                        autoFocus
                      />
                      <button type="submit" className="btn btn--primary btn--sm" disabled={busy}>
                        保存
                      </button>
                      <button
                        type="button"
                        className="btn btn--ghost btn--sm"
                        onClick={() => setEditing(null)}
                        disabled={busy}
                      >
                        取消
                      </button>
                    </form>
                  ) : (
                    <div className="app-card__title">
                      <span className="app-card__name">{app.name}</span>
                      <span className={`badge ${app.enabled ? 'badge--good' : 'badge--warn'}`}>
                        {app.enabled ? '已启用' : '已停用'}
                      </span>
                    </div>
                  )}
                  <dl className="app-card__meta">
                    <div className="app-card__meta-item">
                      <dt className="app-card__meta-label">应用 ID</dt>
                      <dd className="app-card__meta-value">
                        <code>{app.id}</code>
                      </dd>
                    </div>
                    <div className="app-card__meta-item">
                      <dt className="app-card__meta-label">密钥</dt>
                      <dd className="app-card__meta-value">
                        <code>{app.secretHint}…</code>
                      </dd>
                    </div>
                    <div className="app-card__meta-item">
                      <dt className="app-card__meta-label">最近使用</dt>
                      <dd className="app-card__meta-value">
                        {app.lastUsedAt ? formatDateTime(app.lastUsedAt) : '从未使用'}
                      </dd>
                    </div>
                    <div className="app-card__meta-item">
                      <dt className="app-card__meta-label">密钥更新于</dt>
                      <dd className="app-card__meta-value">
                        {formatDateTime(app.secretRotatedAt)}
                      </dd>
                    </div>
                  </dl>
                </div>

                <div className="app-card__actions">
                  {confirming ? (
                    <>
                      <span className="tile__confirm">
                        {confirming === 'rotate'
                          ? '旧密钥将立即失效，确认重置？'
                          : '确认删除该应用？'}
                      </span>
                      <button
                        type="button"
                        className="btn btn--danger btn--sm"
                        onClick={() =>
                          void (confirming === 'rotate' ? handleRotate(app) : handleDelete(app))
                        }
                        disabled={busy}
                      >
                        {busy ? '处理中…' : '确认'}
                      </button>
                      <button
                        type="button"
                        className="btn btn--ghost btn--sm"
                        onClick={() => setPending(null)}
                        disabled={busy}
                      >
                        取消
                      </button>
                    </>
                  ) : (
                    <>
                      <button
                        type="button"
                        className="btn btn--ghost btn--sm"
                        onClick={() => setEditing({ id: app.id, name: app.name })}
                        disabled={busy || renaming}
                      >
                        重命名
                      </button>
                      <button
                        type="button"
                        className="btn btn--ghost btn--sm"
                        onClick={() => void handleToggle(app)}
                        disabled={busy}
                      >
                        {app.enabled ? '停用' : '启用'}
                      </button>
                      <button
                        type="button"
                        className="btn btn--ghost btn--sm"
                        onClick={() => setPending({ id: app.id, action: 'rotate' })}
                        disabled={busy}
                      >
                        重置密钥
                      </button>
                      <button
                        type="button"
                        className="btn btn--ghost btn--sm app-card__delete"
                        onClick={() => setPending({ id: app.id, action: 'delete' })}
                        disabled={busy}
                      >
                        删除
                      </button>
                    </>
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
