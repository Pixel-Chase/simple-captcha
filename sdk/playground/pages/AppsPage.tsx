import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react';
import type { FormEvent, KeyboardEvent as ReactKeyboardEvent } from 'react';
import { API_PREFIX, adminApi, formatDateTime, getErrorMessage, isAdminApiError } from '../api';
import type { AppItem, AppSecretResponse } from '../api';
import { Icon } from '../components/Icon';
import type { IconName } from '../components/Icon';
import { Modal } from '../components/Modal';
import { useCopy, useToast } from '../components/Toast';
import './AppsPage.css';

interface AppsPageProps {
  token: string;
  onSessionExpired: () => void;
}

/** 刚创建或刚重置的密钥：只在弹窗里展示一次，用户点「我已保存」后清除 */
interface RevealedSecret {
  appId: string;
  name: string;
  origins: string[];
  secret: string;
  reason: 'created' | 'rotated';
}

/** 各弹窗的表单状态；应用只记 ID，渲染时从列表取最新数据 */
interface CreateForm {
  name: string;
  origins: string;
  error: string | null;
}

interface RenameForm {
  id: string;
  name: string;
  error: string | null;
}

interface OriginsForm {
  id: string;
  text: string;
  error: string | null;
}

interface ConfirmState {
  id: string;
  action: 'rotate' | 'delete';
  error: string | null;
}

const NAME_MAX_LENGTH = 40;
const ORIGINS_MAX = 20;
/** 新建应用占用的 busy 标记；应用 ID 形如 app_xxx，不会冲突 */
const CREATE_KEY = '#create';
/** 页面底部调试命令里的密钥占位，不展示任何真实密钥 */
const SECRET_PLACEHOLDER = '<应用密钥>';

/** 验证码服务的 origin：内嵌管理后台与服务同源，接入方的 baseURL 就是它 */
const serviceOrigin = () => window.location.origin;

const stateCurl = (secret: string) =>
  `curl -H "Authorization: Bearer ${secret}" \\\n  ${serviceOrigin()}${API_PREFIX}/captchas/states/<second_key>`;

const providerSnippet = (appId: string) =>
  `<CaptchaProvider api={{ baseURL: '${serviceOrigin()}', appId: '${appId}' }}>`;

/** 文本框里每行一个来源（也接受逗号分隔），忽略空行；规范化与校验交给服务端 */
const parseOrigins = (text: string) =>
  text
    .split(/[\n,，]/)
    .map((line) => line.trim())
    .filter(Boolean);

/** 名称首字，作为列表里的应用头像 */
const initialOf = (name: string) => (Array.from(name.trim())[0] ?? '?').toUpperCase();

const tooManyOrigins = `每个应用最多配置 ${ORIGINS_MAX} 个允许来源`;

export function AppsPage({ token, onSessionExpired }: AppsPageProps) {
  const toast = useToast();
  const copy = useCopy();
  const [items, setItems] = useState<AppItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState('');
  const [busyIds, setBusyIds] = useState<ReadonlySet<string>>(() => new Set());
  const [createForm, setCreateForm] = useState<CreateForm | null>(null);
  const [renameForm, setRenameForm] = useState<RenameForm | null>(null);
  const [originsForm, setOriginsForm] = useState<OriginsForm | null>(null);
  const [confirm, setConfirm] = useState<ConfirmState | null>(null);
  const [revealed, setRevealed] = useState<RevealedSecret | null>(null);

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

  /** 加载列表，返回是否成功（手动刷新成功时给轻提示） */
  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await adminApi.listApps(token);
      setItems(data.items);
      return true;
    } catch (err) {
      const message = handleApiError(err, '加载应用失败');
      if (message) {
        setError(message);
      }
      return false;
    } finally {
      setLoading(false);
    }
  }, [token, handleApiError]);

  useEffect(() => {
    void load();
  }, [load]);

  const refresh = async () => {
    if (await load()) {
      toast('应用列表已刷新');
    }
  };

  const findApp = (id: string | undefined) =>
    id ? (items.find((item) => item.id === id) ?? null) : null;

  const replaceItem = (app: AppItem) =>
    setItems((prev) => prev.map((item) => (item.id === app.id ? app : item)));

  const reveal = (result: AppSecretResponse, reason: RevealedSecret['reason']) =>
    setRevealed({
      appId: result.app.id,
      name: result.app.name,
      origins: result.app.origins,
      secret: result.secret,
      reason,
    });

  const setBusy = (id: string, on: boolean) =>
    setBusyIds((prev) => {
      const next = new Set(prev);
      if (on) {
        next.add(id);
      } else {
        next.delete(id);
      }
      return next;
    });

  /** 包一层统一的 busy / 错误处理，返回是否成功；错误交给 onError 展示（弹窗内或轻提示） */
  const run = async (
    id: string,
    fallback: string,
    task: () => Promise<void>,
    onError: (message: string) => void,
  ) => {
    setBusy(id, true);
    try {
      await task();
      return true;
    } catch (err) {
      const message = handleApiError(err, fallback);
      if (message) {
        onError(message);
      }
      return false;
    } finally {
      setBusy(id, false);
    }
  };

  const creating = busyIds.has(CREATE_KEY);

  const handleCreate = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!createForm || creating) {
      return;
    }
    const name = createForm.name.trim();
    if (!name) {
      return;
    }
    const origins = parseOrigins(createForm.origins);
    if (origins.length > ORIGINS_MAX) {
      setCreateForm({ ...createForm, error: tooManyOrigins });
      return;
    }
    setCreateForm({ ...createForm, error: null });
    await run(
      CREATE_KEY,
      '创建应用失败',
      async () => {
        const result = await adminApi.createApp(token, name, origins);
        setItems((prev) => [...prev, result.app]);
        setCreateForm(null);
        reveal(result, 'created');
      },
      (message) => setCreateForm((prev) => (prev ? { ...prev, error: message } : prev)),
    );
  };

  const renameApp = findApp(renameForm?.id);

  const handleRename = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!renameForm || !renameApp || busyIds.has(renameApp.id)) {
      return;
    }
    const name = renameForm.name.trim();
    if (!name) {
      return;
    }
    if (name === renameApp.name) {
      setRenameForm(null);
      return;
    }
    await run(
      renameApp.id,
      '重命名失败',
      async () => {
        const updated = await adminApi.updateApp(token, renameApp.id, {
          name,
          enabled: renameApp.enabled,
        });
        replaceItem(updated);
        setRenameForm(null);
        toast(`已重命名为「${updated.name}」`);
      },
      (message) => setRenameForm((prev) => (prev ? { ...prev, error: message } : prev)),
    );
  };

  const originsApp = findApp(originsForm?.id);

  const handleSaveOrigins = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!originsForm || !originsApp || busyIds.has(originsApp.id)) {
      return;
    }
    const origins = parseOrigins(originsForm.text);
    if (origins.length > ORIGINS_MAX) {
      setOriginsForm({ ...originsForm, error: tooManyOrigins });
      return;
    }
    setOriginsForm({ ...originsForm, error: null });
    await run(
      originsApp.id,
      '保存允许来源失败',
      async () => {
        const updated = await adminApi.updateApp(token, originsApp.id, {
          name: originsApp.name,
          enabled: originsApp.enabled,
          origins,
        });
        replaceItem(updated);
        setOriginsForm(null);
        toast(
          updated.origins.length > 0
            ? `「${updated.name}」的允许来源已保存，立即生效`
            : `已清空「${updated.name}」的允许来源`,
        );
      },
      (message) => setOriginsForm((prev) => (prev ? { ...prev, error: message } : prev)),
    );
  };

  /** 启停直接执行，结果用轻提示反馈 */
  const handleToggle = (app: AppItem) =>
    run(
      app.id,
      app.enabled ? '停用失败' : '启用失败',
      async () => {
        const updated = await adminApi.updateApp(token, app.id, {
          name: app.name,
          enabled: !app.enabled,
        });
        replaceItem(updated);
        toast(
          updated.enabled
            ? `已启用「${updated.name}」`
            : `已停用「${updated.name}」，出题、提交与二次校验都会被拒绝`,
        );
      },
      (message) => toast(message, 'error'),
    );

  const confirmApp = findApp(confirm?.id);

  const handleConfirm = async () => {
    if (!confirm || !confirmApp || busyIds.has(confirmApp.id)) {
      return;
    }
    const app = confirmApp;
    const onError = (message: string) =>
      setConfirm((prev) => (prev ? { ...prev, error: message } : prev));
    setConfirm({ ...confirm, error: null });
    if (confirm.action === 'rotate') {
      await run(
        app.id,
        '重置密钥失败',
        async () => {
          const result = await adminApi.rotateAppSecret(token, app.id);
          replaceItem(result.app);
          setConfirm(null);
          reveal(result, 'rotated');
        },
        onError,
      );
      return;
    }
    await run(
      app.id,
      '删除失败',
      async () => {
        await adminApi.deleteApp(token, app.id);
        setItems((prev) => prev.filter((item) => item.id !== app.id));
        setConfirm(null);
        toast(`已删除「${app.name}」`);
      },
      onError,
    );
  };

  const openOrigins = (app: AppItem) =>
    setOriginsForm({ id: app.id, text: app.origins.join('\n'), error: null });

  const userApps = items.filter((app) => !app.isSystem);
  const enabledCount = userApps.filter((app) => app.enabled).length;
  const keyword = query.trim().toLowerCase();
  const visible = useMemo(
    () =>
      keyword
        ? items.filter(
            (app) =>
              app.name.toLowerCase().includes(keyword) || app.id.toLowerCase().includes(keyword),
          )
        : items,
    [items, keyword],
  );
  const showInitialState = items.length === 0 && (loading || !!error);
  const debugCurl = stateCurl(SECRET_PLACEHOLDER);

  return (
    <section className="page apps">
      <div className="page-head">
        <div>
          <div className="page-head__title">
            <h1 className="page-title">接入应用</h1>
            {!showInitialState && (
              <span className="chip apps__count">共 {userApps.length} 个应用</span>
            )}
          </div>
          <p className="page-desc">
            管理接入验证码的前端站点与业务后端密钥。每个接入站点对应一个应用，只有「允许来源」里的页面能出题与提交。
          </p>
        </div>
        <div className="page-head__actions">
          <button type="button" className="btn" onClick={() => void refresh()} disabled={loading}>
            <Icon name="refresh" size={16} className={loading ? 'spin' : undefined} />
            刷新列表
          </button>
          <button
            type="button"
            className="btn btn--primary"
            onClick={() => setCreateForm({ name: '', origins: '', error: null })}
          >
            <Icon name="plus" size={16} />
            新建应用
          </button>
        </div>
      </div>

      <section className="panel apps__guide" aria-labelledby="apps-guide-title">
        <div className="apps__guide-head">
          <span className="apps__guide-icon" aria-hidden="true">
            <Icon name="code" size={18} />
          </span>
          <div>
            <h2 id="apps-guide-title" className="apps__guide-title">
              接入方式与密钥安全
            </h2>
            <p className="apps__guide-desc">
              前端凭应用 ID 出题，业务后端凭应用密钥做二次校验，两种凭据各司其职：
            </p>
          </div>
        </div>
        <div className="apps__guide-grid">
          <div className="apps__guide-item">
            <div className="apps__guide-step">
              <span className="apps__guide-num">1</span>
              前端出题
            </div>
            <p>
              前端 <code>CaptchaProvider</code> 传入应用 ID，只有「允许来源」中的页面能出题与提交。
            </p>
          </div>
          <div className="apps__guide-item">
            <div className="apps__guide-step">
              <span className="apps__guide-num">2</span>
              二次校验
            </div>
            <p>
              业务后端携带 <code>Authorization: Bearer &lt;应用密钥&gt;</code> 调用{' '}
              <code>GET {API_PREFIX}/captchas/states/:key</code> 确认验证结果。
            </p>
          </div>
          <div className="apps__guide-item">
            <div className="apps__guide-step apps__guide-step--warn">
              <span className="apps__guide-num">!</span>
              密钥安全
            </div>
            <p>
              密钥只在创建或重置时完整显示一次，服务端只保存哈希；停用、重置或删除后旧密钥立即失效。
            </p>
          </div>
        </div>
      </section>

      {error && !showInitialState && (
        <div className="alert alert--error" role="alert">
          <Icon name="error" size={16} />
          <span>{error}</span>
        </div>
      )}

      <div className="panel panel--flush apps__list">
        <div className="apps__toolbar">
          <div className="apps__toolbar-title">
            <span className="apps__toolbar-name">接入列表</span>
            <span className="apps__toolbar-sep" aria-hidden="true">
              |
            </span>
            <span className="chip">系统应用在前，其余按创建时间排列</span>
          </div>
          <label className="apps__search">
            <Icon name="search" size={16} />
            <input
              type="search"
              className="apps__search-input"
              placeholder="按应用名称或应用 ID 过滤"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              aria-label="按应用名称或应用 ID 过滤"
            />
          </label>
        </div>

        {showInitialState ? (
          loading ? (
            <div className="apps__state">
              <Icon name="spinner" size={20} className="spin" />
              正在加载应用…
            </div>
          ) : (
            <div className="apps__state apps__state--error" role="alert">
              <Icon name="error" size={20} />
              <span>{error}</span>
              <button type="button" className="btn btn--sm" onClick={() => void load()}>
                <Icon name="refresh" size={14} />
                重试
              </button>
            </div>
          )
        ) : (
          <>
            <div className="apps__table-wrap">
              <table className="table apps-table">
                <thead>
                  <tr>
                    <th scope="col">应用名称</th>
                    <th scope="col">状态</th>
                    <th scope="col">应用 ID</th>
                    <th scope="col">密钥提示</th>
                    <th scope="col">允许来源</th>
                    <th scope="col" title="业务后端最近一次用该应用密钥调用二次校验的时间">
                      最近使用
                    </th>
                    <th scope="col">密钥更新于</th>
                    <th scope="col" className="apps-table__actions-head">
                      操作
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {visible.map((app) => (
                    <AppRow
                      key={app.id}
                      app={app}
                      busy={busyIds.has(app.id)}
                      onCopyId={() => void copy(app.id, '已复制应用 ID')}
                      onEditOrigins={() => openOrigins(app)}
                      onRename={() => setRenameForm({ id: app.id, name: app.name, error: null })}
                      onToggle={() => void handleToggle(app)}
                      onRotate={() => setConfirm({ id: app.id, action: 'rotate', error: null })}
                      onDelete={() => setConfirm({ id: app.id, action: 'delete', error: null })}
                    />
                  ))}
                  {keyword && visible.length === 0 && (
                    <tr>
                      <td colSpan={8} className="apps-table__no-match">
                        没有名称或 ID 包含「{query.trim()}」的应用
                        <button
                          type="button"
                          className="btn btn--text"
                          onClick={() => setQuery('')}
                        >
                          清除过滤
                        </button>
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>

            {userApps.length === 0 && !keyword && (
              <div className="apps__empty">
                <span className="apps__empty-icon" aria-hidden="true">
                  <Icon name="layers" size={22} />
                </span>
                <strong className="apps__empty-title">还没有接入应用</strong>
                <p className="apps__empty-text">
                  为每个接入验证码的站点建一个应用并填写允许来源；创建之前，业务后端的二次校验会一律返回
                  401。
                </p>
                <button
                  type="button"
                  className="btn btn--primary btn--sm"
                  onClick={() => setCreateForm({ name: '', origins: '', error: null })}
                >
                  <Icon name="plus" size={14} />
                  新建应用
                </button>
              </div>
            )}

            <div className="apps__foot">
              <div className="apps__stats">
                <span>
                  共 <strong className="apps__stat">{userApps.length}</strong> 个应用
                </span>
                <span className="apps__stats-dot" aria-hidden="true" />
                <span>
                  启用 <strong className="apps__stat apps__stat--good">{enabledCount}</strong>
                </span>
                <span className="apps__stats-dot" aria-hidden="true" />
                <span>
                  停用 <strong className="apps__stat">{userApps.length - enabledCount}</strong>
                </span>
                {keyword && (
                  <>
                    <span className="apps__stats-dot" aria-hidden="true" />
                    <span>当前显示 {visible.length} 个</span>
                  </>
                )}
              </div>
              <p className="apps__foot-note">来源按请求头 Origin 精确匹配，修改后立即生效。</p>
            </div>
          </>
        )}
      </div>

      <section className="apps__curl" aria-labelledby="apps-curl-title">
        <div className="apps__curl-head">
          <h2 id="apps-curl-title" className="apps__curl-title">
            <Icon name="terminal" size={18} />
            二次校验调试 cURL 命令（业务后端示例）
          </h2>
          <span className="apps__curl-hint">请求头 Authorization: Bearer {SECRET_PLACEHOLDER}</span>
        </div>
        <div className="apps__curl-body">
          <code className="apps__curl-code">{debugCurl}</code>
          <button
            type="button"
            className="apps__curl-copy"
            onClick={() => void copy(debugCurl, '已复制校验命令')}
          >
            <Icon name="copy" size={14} />
            复制
          </button>
        </div>
        <p className="apps__curl-note">
          把 <code>{SECRET_PLACEHOLDER}</code> 换成应用密钥、<code>&lt;second_key&gt;</code>{' '}
          换成前端验证成功回调里的 <code>data.second_key</code>；它只能用出题应用的密钥查询。
        </p>
      </section>

      {createForm && (
        <Modal
          open
          onClose={() => setCreateForm(null)}
          size="md"
          icon="plus"
          title="新建应用"
          subtitle="每个接入验证码的站点一个应用，创建后只显示一次应用密钥。"
          dismissible={!creating}
          footer={
            <>
              <button
                type="button"
                className="btn"
                onClick={() => setCreateForm(null)}
                disabled={creating}
              >
                取消
              </button>
              <button
                type="submit"
                form="apps-create-form"
                className="btn btn--primary"
                disabled={creating || !createForm.name.trim()}
              >
                {creating ? '创建中…' : '创建应用'}
              </button>
            </>
          }
        >
          <form id="apps-create-form" className="apps-form" onSubmit={handleCreate}>
            <div className="field">
              <div className="apps-form__label-row">
                <label className="field__label" htmlFor="apps-create-name">
                  应用名称
                </label>
                <span className="apps-form__counter">
                  {Array.from(createForm.name).length} / {NAME_MAX_LENGTH}
                </span>
              </div>
              <input
                id="apps-create-name"
                className="field__input"
                placeholder="例如：订单系统"
                value={createForm.name}
                maxLength={NAME_MAX_LENGTH}
                onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })}
                disabled={creating}
                // biome-ignore lint/a11y/noAutofocus: 打开「新建应用」后直接输入名称
                autoFocus
              />
              <span className="field__hint">不能与已有应用重名。</span>
            </div>
            <div className="field">
              <div className="apps-form__label-row">
                <label className="field__label" htmlFor="apps-create-origins">
                  允许来源
                </label>
                <OriginsCount text={createForm.origins} />
              </div>
              <textarea
                id="apps-create-origins"
                className="field__input field__input--mono apps-form__textarea"
                placeholder={'每行一个，例如：\nhttps://www.example.com\nhttp://localhost:3000'}
                rows={4}
                value={createForm.origins}
                onChange={(e) => setCreateForm({ ...createForm, origins: e.target.value })}
                disabled={creating}
              />
              <span className="field__hint">
                前端页面的 origin（协议 + 域名 +
                端口，不含路径），只做精确匹配。可以先留空、之后再配置，留空期间前端出题与提交会被拒绝。
              </span>
            </div>
            {createForm.error && (
              <div className="alert alert--error" role="alert">
                <Icon name="error" size={16} />
                <span>{createForm.error}</span>
              </div>
            )}
          </form>
        </Modal>
      )}

      {renameForm && renameApp && (
        <Modal
          open
          onClose={() => setRenameForm(null)}
          size="sm"
          icon="file-text"
          title="重命名应用"
          subtitle={renameApp.id}
          dismissible={!busyIds.has(renameApp.id)}
          footer={
            <>
              <button
                type="button"
                className="btn"
                onClick={() => setRenameForm(null)}
                disabled={busyIds.has(renameApp.id)}
              >
                取消
              </button>
              <button
                type="submit"
                form="apps-rename-form"
                className="btn btn--primary"
                disabled={busyIds.has(renameApp.id) || !renameForm.name.trim()}
              >
                {busyIds.has(renameApp.id) ? '保存中…' : '保存'}
              </button>
            </>
          }
        >
          <form id="apps-rename-form" className="apps-form" onSubmit={handleRename}>
            <div className="field">
              <div className="apps-form__label-row">
                <label className="field__label" htmlFor="apps-rename-name">
                  应用名称
                </label>
                <span className="apps-form__counter">
                  {Array.from(renameForm.name).length} / {NAME_MAX_LENGTH}
                </span>
              </div>
              <input
                id="apps-rename-name"
                className="field__input"
                value={renameForm.name}
                maxLength={NAME_MAX_LENGTH}
                onChange={(e) => setRenameForm({ ...renameForm, name: e.target.value })}
                disabled={busyIds.has(renameApp.id)}
                // biome-ignore lint/a11y/noAutofocus: 打开「重命名」后直接编辑
                autoFocus
              />
              <span className="field__hint">不能与其他应用重名；应用 ID 与密钥不变。</span>
            </div>
            {renameForm.error && (
              <div className="alert alert--error" role="alert">
                <Icon name="error" size={16} />
                <span>{renameForm.error}</span>
              </div>
            )}
          </form>
        </Modal>
      )}

      {originsForm && originsApp && (
        <OriginsDrawer
          app={originsApp}
          form={originsForm}
          busy={busyIds.has(originsApp.id)}
          onChange={(text) => setOriginsForm({ ...originsForm, text, error: null })}
          onSubmit={handleSaveOrigins}
          onClose={() => setOriginsForm(null)}
        />
      )}

      {confirm && confirmApp && (
        <ConfirmDialog
          app={confirmApp}
          state={confirm}
          busy={busyIds.has(confirmApp.id)}
          onConfirm={() => void handleConfirm()}
          onClose={() => setConfirm(null)}
        />
      )}

      {revealed && (
        <SecretDialog
          revealed={revealed}
          onCopy={(text, done) => void copy(text, done)}
          onDone={() => setRevealed(null)}
        />
      )}
    </section>
  );
}

interface AppRowProps {
  app: AppItem;
  busy: boolean;
  onCopyId: () => void;
  onEditOrigins: () => void;
  onRename: () => void;
  onToggle: () => void;
  onRotate: () => void;
  onDelete: () => void;
}

/** 来源超过这个数时先折叠，避免一行撑得过高 */
const ORIGINS_PREVIEW = 4;

/** 列表的一行：系统应用只读，普通应用主操作是编辑来源，其余放进「更多」菜单 */
function AppRow({
  app,
  busy,
  onCopyId,
  onEditOrigins,
  onRename,
  onToggle,
  onRotate,
  onDelete,
}: AppRowProps) {
  const [expanded, setExpanded] = useState(false);
  const disabled = !app.isSystem && !app.enabled;
  const collapsed = !expanded && app.origins.length > ORIGINS_PREVIEW;
  const shownOrigins = collapsed ? app.origins.slice(0, ORIGINS_PREVIEW - 1) : app.origins;

  return (
    <tr className={disabled ? 'apps-row apps-row--disabled' : 'apps-row'}>
      <td>
        <div className="apps-row__app">
          {app.isSystem ? (
            <span className="apps-row__avatar apps-row__avatar--system" aria-hidden="true">
              <Icon name="terminal" size={14} />
            </span>
          ) : (
            <span className="apps-row__avatar" aria-hidden="true">
              {initialOf(app.name)}
            </span>
          )}
          <div className="apps-row__app-text">
            <div className="apps-row__name-line">
              <span className="apps-row__name">{app.name}</span>
              {app.isSystem && (
                <span className="chip chip--primary apps-row__system">系统应用</span>
              )}
            </div>
            {app.isSystem && <span className="apps-row__sub">管理后台自己的验证码</span>}
          </div>
        </div>
      </td>

      <td>
        {app.enabled ? (
          <span className={app.isSystem ? 'chip apps-row__status' : 'chip chip--good'}>
            <span className="chip__dot apps-row__dot--good" />
            已启用
          </span>
        ) : (
          <>
            <span className="chip">
              <span className="chip__dot" />
              已停用
            </span>
            <span className="apps-row__note">出题与校验均被拒绝</span>
          </>
        )}
      </td>

      <td>
        <div className="apps-row__id">
          <span className="apps-row__id-text">{app.id}</span>
          <button
            type="button"
            className="btn btn--ghost btn--icon apps-row__copy"
            onClick={onCopyId}
            title="复制应用 ID"
            aria-label={`复制「${app.name}」的应用 ID`}
          >
            <Icon name="copy" size={14} />
          </button>
        </div>
      </td>

      <td>
        {app.isSystem ? (
          <div className="apps-row__hint apps-row__hint--none">
            <span>
              无密钥 <span className="apps-row__hint-extra">· 仅同源</span>
            </span>
            <span className="apps-row__sub">不能用于二次校验</span>
          </div>
        ) : (
          <div className="apps-row__hint">
            <Icon name={app.enabled ? 'shield-lock' : 'lock'} size={13} />
            <span className="apps-row__hint-text">{app.secretHint}…</span>
            <span
              className="apps-row__hint-info"
              title="完整密钥只在创建或重置时显示一次，遗失请重置"
              role="img"
              aria-label="完整密钥只在创建或重置时显示一次，遗失请重置"
            >
              <Icon name="info" size={13} />
            </span>
          </div>
        )}
      </td>

      <td className="apps-row__origins-cell">
        {app.isSystem ? (
          <span className="apps-row__origin apps-row__origin--locked">
            <Icon name="lock" size={11} />
            仅同源<span className="apps-row__origin-extra">（内置）</span>
          </span>
        ) : app.origins.length === 0 ? (
          <span className="apps-row__origins-empty">
            <Icon name="warning" size={12} />
            未配置允许来源（前端出题与提交会被拒绝）
          </span>
        ) : (
          <div className="apps-row__origins">
            {shownOrigins.map((origin) => (
              <span key={origin} className="apps-row__origin">
                <Icon name={origin.startsWith('https:') ? 'lock' : 'globe'} size={11} />
                {origin}
              </span>
            ))}
            {app.origins.length > ORIGINS_PREVIEW && (
              <button
                type="button"
                className="apps-row__origins-more"
                onClick={() => setExpanded((value) => !value)}
                aria-expanded={expanded}
              >
                {expanded ? '收起' : `还有 ${app.origins.length - shownOrigins.length} 个`}
              </button>
            )}
          </div>
        )}
      </td>

      <td className="apps-row__time">
        {app.isSystem ? (
          <span className="apps-row__muted">—</span>
        ) : app.lastUsedAt ? (
          formatDateTime(app.lastUsedAt)
        ) : (
          <span className="apps-row__never">从未使用</span>
        )}
      </td>

      <td className="apps-row__time">
        {app.isSystem ? (
          <span className="apps-row__muted" title="系统应用没有密钥">
            —
          </span>
        ) : (
          formatDateTime(app.secretRotatedAt)
        )}
      </td>

      <td className="apps-row__actions-cell">
        {app.isSystem ? (
          <span
            className="apps-row__locked"
            title="系统应用不可改名、停用、重置密钥、修改来源或删除"
          >
            <Icon name="lock" size={12} />
            不可操作
          </span>
        ) : (
          <div className="apps-row__actions">
            {app.origins.length === 0 ? (
              <button
                type="button"
                className="btn btn--primary btn--sm apps-row__primary"
                onClick={onEditOrigins}
                disabled={busy}
              >
                配置来源
              </button>
            ) : (
              <button
                type="button"
                className="btn btn--text"
                onClick={onEditOrigins}
                disabled={busy}
              >
                编辑来源
              </button>
            )}
            <RowMenu
              label={`「${app.name}」的更多操作`}
              busy={busy}
              items={[
                { key: 'rename', label: '重命名', icon: 'file-text', onSelect: onRename },
                app.enabled
                  ? { key: 'toggle', label: '停用', icon: 'x-circle', onSelect: onToggle }
                  : { key: 'toggle', label: '启用', icon: 'check-circle', onSelect: onToggle },
                { key: 'rotate', label: '重置密钥', icon: 'key', onSelect: onRotate },
                { key: 'delete', label: '删除', icon: 'trash', danger: true, onSelect: onDelete },
              ]}
            />
          </div>
        )}
      </td>
    </tr>
  );
}

interface RowMenuItem {
  key: string;
  label: string;
  icon: IconName;
  danger?: boolean;
  onSelect: () => void;
}

/** 菜单的大致高度，用来判断向下展开是否会超出视口 */
const MENU_HEIGHT = 190;

type MenuPosition = { right: number; top?: number; bottom?: number };

/**
 * 行内「更多」菜单：锚定在触发按钮旁。表格容器会横向滚动（overflow 会裁掉绝对定位的弹层），
 * 所以面板用 fixed 定位、打开时按按钮位置计算坐标；滚动、缩放窗口、点外部或按 Esc 时关闭
 */
function RowMenu({ label, items, busy }: { label: string; items: RowMenuItem[]; busy: boolean }) {
  const [position, setPosition] = useState<MenuPosition | null>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const menuId = useId();
  const open = position !== null;

  const close = useCallback((restoreFocus: boolean) => {
    setPosition(null);
    if (restoreFocus) {
      triggerRef.current?.focus();
    }
  }, []);

  const toggle = () => {
    const trigger = triggerRef.current;
    if (open || !trigger) {
      setPosition(null);
      return;
    }
    const rect = trigger.getBoundingClientRect();
    const { clientWidth, clientHeight } = document.documentElement;
    const spaceBelow = clientHeight - rect.bottom;
    const upward = spaceBelow < MENU_HEIGHT && rect.top > spaceBelow;
    setPosition({
      right: Math.max(8, clientWidth - rect.right),
      ...(upward ? { bottom: clientHeight - rect.top + 4 } : { top: rect.bottom + 4 }),
    });
  };

  useEffect(() => {
    if (!open) {
      return;
    }
    panelRef.current
      ?.querySelector<HTMLElement>('[role="menuitem"]')
      ?.focus({ preventScroll: true });

    const onPointerDown = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) {
        close(false);
      }
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.stopPropagation();
        close(true);
      } else if (event.key === 'Tab') {
        close(false);
      }
    };
    // 面板是 fixed 定位，页面或表格滚动后会与按钮错位，直接关闭
    const onScroll = () => close(false);
    document.addEventListener('pointerdown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    window.addEventListener('scroll', onScroll, true);
    window.addEventListener('resize', onScroll);
    return () => {
      document.removeEventListener('pointerdown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
      window.removeEventListener('scroll', onScroll, true);
      window.removeEventListener('resize', onScroll);
    };
  }, [open, close]);

  /** 上下方向键在菜单项之间移动焦点 */
  const onPanelKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') {
      return;
    }
    event.preventDefault();
    const list = Array.from(
      panelRef.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [],
    );
    const index = list.indexOf(document.activeElement as HTMLElement);
    const step = event.key === 'ArrowDown' ? 1 : -1;
    list[(index + step + list.length) % list.length]?.focus();
  };

  return (
    <div className="apps-menu" ref={rootRef}>
      <button
        ref={triggerRef}
        type="button"
        className="btn btn--ghost btn--icon apps-menu__trigger"
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        onClick={toggle}
        disabled={busy}
      >
        <Icon name={busy ? 'spinner' : 'more'} size={18} className={busy ? 'spin' : undefined} />
      </button>
      {position && (
        <div
          ref={panelRef}
          id={menuId}
          className="apps-menu__panel"
          role="menu"
          aria-label={label}
          style={position}
          onKeyDown={onPanelKeyDown}
        >
          {items.map((item) => (
            <button
              key={item.key}
              type="button"
              role="menuitem"
              className={
                item.danger ? 'apps-menu__item apps-menu__item--danger' : 'apps-menu__item'
              }
              onClick={() => {
                close(true);
                item.onSelect();
              }}
            >
              <Icon name={item.icon} size={16} />
              {item.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

/** 来源条数：超过上限时标红，提交前就能看到 */
function OriginsCount({ text }: { text: string }) {
  const count = parseOrigins(text).length;
  const tone = count > ORIGINS_MAX ? 'over' : count === 0 ? 'empty' : 'ok';
  return (
    <span className={`apps-count apps-count--${tone}`} aria-live="polite">
      {count} / {ORIGINS_MAX}
    </span>
  );
}

interface OriginsDrawerProps {
  app: AppItem;
  form: OriginsForm;
  busy: boolean;
  onChange: (text: string) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onClose: () => void;
}

/** 编辑允许来源：右侧抽屉，书写规则与服务端 NormalizeOrigin 一致 */
function OriginsDrawer({ app, form, busy, onChange, onSubmit, onClose }: OriginsDrawerProps) {
  const empty = parseOrigins(form.text).length === 0;
  return (
    <Modal
      open
      onClose={onClose}
      variant="drawer"
      icon="globe"
      title="编辑允许来源"
      subtitle={`${app.name}（${app.id}）`}
      dismissible={!busy}
      footer={
        <>
          <button type="button" className="btn" onClick={onClose} disabled={busy}>
            取消
          </button>
          <button
            type="submit"
            form="apps-origins-form"
            className="btn btn--primary"
            disabled={busy}
          >
            {busy ? '保存中…' : '保存并立即生效'}
          </button>
        </>
      }
    >
      <form id="apps-origins-form" className="apps-drawer" onSubmit={onSubmit}>
        <div className="apps-drawer__intro">
          <p className="apps-drawer__intro-title">来源匹配说明</p>
          <p>
            每行填写一个前端页面的 origin（也可以用逗号分隔），按协议 + 域名 +
            端口精确匹配，保存后立即生效。
          </p>
        </div>

        <div className="field">
          <div className="apps-form__label-row">
            <label className="field__label" htmlFor="apps-origins-input">
              允许来源
            </label>
            <OriginsCount text={form.text} />
          </div>
          <textarea
            id="apps-origins-input"
            className="field__input field__input--mono apps-drawer__textarea"
            placeholder={'https://example.com\nhttps://m.example.com\nhttp://localhost:3000'}
            rows={7}
            value={form.text}
            onChange={(e) => onChange(e.target.value)}
            disabled={busy}
            // biome-ignore lint/a11y/noAutofocus: 打开抽屉后直接编辑
            autoFocus
          />
          <div className="apps-drawer__meta">
            <span>只支持 http:// 与 https://</span>
            <span>最多 {ORIGINS_MAX} 个</span>
          </div>
        </div>

        <div className="apps-drawer__rules">
          <p className="apps-drawer__rules-title">
            <Icon name="list-check" size={14} />
            书写规则
          </p>
          <ul>
            <li>
              <strong>可以写本地地址：</strong>
              <code>http://localhost:3000</code>、<code>http://127.0.0.1:8080</code>
            </li>
            <li>
              <strong>非默认端口要写上：</strong>
              <code>https://app.example.com:8443</code>；80 / 443 默认端口会自动省略
            </li>
            <li>
              <strong className="apps-drawer__no">不能包含路径或查询串：</strong>
              <code className="apps-drawer__bad">https://example.com/login</code>
            </li>
            <li>
              <strong className="apps-drawer__no">不支持通配符：</strong>
              <code className="apps-drawer__bad">https://*.example.com</code>
            </li>
            <li>保存时统一转为小写并去掉重复项。</li>
          </ul>
        </div>

        <div className={empty ? 'alert alert--error' : 'alert alert--warn'}>
          <Icon name={empty ? 'warning' : 'info'} size={16} />
          <span>
            {empty
              ? '当前没有填写任何来源：保存后该应用的前端出题与提交会被拒绝。'
              : '清空全部来源后，该应用的前端出题与提交会被拒绝。'}
          </span>
        </div>

        {form.error && (
          <div className="alert alert--error" role="alert">
            <Icon name="error" size={16} />
            <span>{form.error}</span>
          </div>
        )}
      </form>
    </Modal>
  );
}

interface ConfirmDialogProps {
  app: AppItem;
  state: ConfirmState;
  busy: boolean;
  onConfirm: () => void;
  onClose: () => void;
}

/** 重置密钥 / 删除应用的危险确认 */
function ConfirmDialog({ app, state, busy, onConfirm, onClose }: ConfirmDialogProps) {
  const rotate = state.action === 'rotate';
  return (
    <Modal
      open
      onClose={onClose}
      size="sm"
      tone="danger"
      icon="warning"
      title={rotate ? '重置应用密钥' : '删除应用'}
      subtitle={
        <>
          你正在{rotate ? '为' : '删除'}{' '}
          <strong className="apps-confirm__target">{app.name}</strong>（{app.id}）
          {rotate ? '重新生成密钥' : ''}。
        </>
      }
      dismissible={!busy}
      footer={
        <>
          <button type="button" className="btn" onClick={onClose} disabled={busy}>
            取消
          </button>
          <button type="button" className="btn btn--danger" onClick={onConfirm} disabled={busy}>
            {busy ? '处理中…' : rotate ? '确认重置密钥' : '确认删除'}
          </button>
        </>
      }
    >
      <div className="apps-confirm">
        <p className="apps-confirm__title">操作后果</p>
        {rotate ? (
          <ul>
            <li>
              <strong>旧密钥立即失效</strong>，无法恢复。
            </li>
            <li>
              仍用旧密钥调用二次校验的业务后端会被拒绝（返回 <code>401</code>）。
            </li>
            <li>新密钥只在接下来的弹窗中完整显示一次，请及时更新业务后端的配置。</li>
          </ul>
        ) : (
          <ul>
            <li>
              <strong>删除后不可恢复</strong>，应用 ID 与密钥立即失效。
            </li>
            <li>该应用的前端出题与提交会被拒绝，业务后端的二次校验也会被拒绝。</li>
          </ul>
        )}
      </div>
      {state.error && (
        <div className="alert alert--error apps-confirm__error" role="alert">
          <Icon name="error" size={16} />
          <span>{state.error}</span>
        </div>
      )}
    </Modal>
  );
}

interface SecretDialogProps {
  revealed: RevealedSecret;
  onCopy: (text: string, done: string) => void;
  onDone: () => void;
}

/** 一次性密钥：不能点遮罩或 Esc 关闭，必须点「我已保存」 */
function SecretDialog({ revealed, onCopy, onDone }: SecretDialogProps) {
  const rotated = revealed.reason === 'rotated';
  const snippet = providerSnippet(revealed.appId);
  const curl = stateCurl(revealed.secret);
  return (
    <Modal
      open
      onClose={onDone}
      size="lg"
      tone="success"
      icon="check-circle"
      title={rotated ? '密钥已重置' : '应用创建成功'}
      subtitle={
        rotated
          ? '已为该应用生成新密钥，请用它替换业务后端的配置。'
          : '应用已生效，请复制密钥并配置到业务后端。'
      }
      dismissible={false}
      footer={
        <button type="button" className="btn btn--primary apps-secret__done" onClick={onDone}>
          <Icon name="check-circle" size={16} />
          我已保存
        </button>
      }
    >
      <div className="apps-secret">
        <div className="apps-secret__warn" role="alert">
          <Icon name="warning" size={20} />
          <div>
            <p className="apps-secret__warn-main">
              密钥只显示这一次。请立即复制并妥善保管：服务端只保存哈希，关闭本窗口后无法再次查看，遗失只能重置。
            </p>
            {rotated && (
              <p className="apps-secret__warn-extra">
                <Icon name="info" size={14} />
                旧密钥已经失效，仍在使用旧密钥的业务后端会被拒绝。
              </p>
            )}
          </div>
        </div>

        <dl className="apps-secret__meta">
          <div>
            <dt>应用名称</dt>
            <dd className="apps-secret__meta-name">{revealed.name}</dd>
          </div>
          <div>
            <dt>应用 ID</dt>
            <dd className="apps-secret__meta-id">
              <span>{revealed.appId}</span>
              <button
                type="button"
                className="btn btn--ghost btn--icon apps-row__copy"
                onClick={() => onCopy(revealed.appId, '已复制应用 ID')}
                title="复制应用 ID"
                aria-label="复制应用 ID"
              >
                <Icon name="copy" size={14} />
              </button>
            </dd>
          </div>
          <div className="apps-secret__meta-wide">
            <dt>允许来源</dt>
            {revealed.origins.length > 0 ? (
              <dd className="apps-secret__meta-origins">
                {revealed.origins.map((origin) => (
                  <span key={origin}>{origin}</span>
                ))}
              </dd>
            ) : (
              <dd className="apps-secret__meta-empty">
                未配置（前端出题与提交会被拒绝），可稍后在列表中「配置来源」
              </dd>
            )}
          </div>
        </dl>

        <div className="apps-secret__block">
          <div className="apps-secret__label-row">
            <span className="apps-secret__label apps-secret__label--strong">
              <Icon name="key" size={14} />
              应用密钥（一次性明文）
            </span>
            <span className="apps-secret__label-warn">关闭后无法再次查看</span>
          </div>
          <div className="apps-secret__key">
            <code className="apps-secret__key-text">{revealed.secret}</code>
            <button
              type="button"
              className="apps-secret__key-copy"
              onClick={() => onCopy(revealed.secret, '已复制密钥')}
            >
              <Icon name="copy" size={14} />
              复制密钥
            </button>
          </div>
        </div>

        <div className="apps-secret__block">
          <div className="apps-secret__label-row">
            <span className="apps-secret__label">前端接入代码（React SDK）</span>
            <button
              type="button"
              className="apps-secret__copy"
              onClick={() => onCopy(snippet, '已复制接入代码')}
            >
              <Icon name="copy" size={14} />
              复制
            </button>
          </div>
          <pre className="apps-secret__snippet">{snippet}</pre>
        </div>

        <div className="apps-secret__block">
          <div className="apps-secret__label-row">
            <span className="apps-secret__label">业务后端二次校验（cURL 调试）</span>
            <button
              type="button"
              className="apps-secret__copy"
              onClick={() => onCopy(curl, '已复制校验命令')}
            >
              <Icon name="copy" size={14} />
              复制
            </button>
          </div>
          <pre className="apps-secret__snippet">{curl}</pre>
        </div>
      </div>
    </Modal>
  );
}
