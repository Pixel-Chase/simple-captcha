/**
 * 无头浏览器与 API 篡改痕迹检测。
 *
 * 检测思路参考 BotD（MIT，FingerprintJS, Inc.）与 CreepJS（MIT，Abraham Juliot）公开的检测点，
 * 代码为本项目独立实现，许可声明见 sdk/THIRD_PARTY_NOTICES.md。
 *
 * 设计约束：
 * - 所有检测只产出负面标记：命中由服务端加难，不命中不影响难度；
 * - 并行执行、总预算内未完成的检测直接放弃、任何异常静默吞掉，真人不会因此多等；
 * - 不调用会弹出权限请求的 API，不采集 Canvas / Audio 指纹。
 */

const MAX_ANOMALIES = 16;
const MAX_MARKER_LENGTH = 64;
/** Worker 一致性检测的单独超时，低于总预算以便其余检测结果能按时返回 */
const WORKER_TIMEOUT_MS = 100;

const PATCHED_GETTER_PROPS = [
  'webdriver',
  'plugins',
  'languages',
  'hardwareConcurrency',
  'userAgent',
];
const NATIVE_CODE_PATTERN = /\{\s*\[native code\]\s*\}\s*$/;

type Detector = () => string[] | Promise<string[]>;

let cachedAnomalies: string[] | null = null;

/** 在 ms 内未完成则返回 fallback，不会抛错 */
function withTimeout<T>(promise: Promise<T>, ms: number, fallback: T): Promise<T> {
  return new Promise<T>((resolve) => {
    const timer = setTimeout(() => resolve(fallback), ms);
    promise.then(
      (value) => {
        clearTimeout(timer);
        resolve(value);
      },
      () => {
        clearTimeout(timer);
        resolve(fallback);
      },
    );
  });
}

function run(detector: Detector): Promise<string[]> {
  return Promise.resolve()
    .then(detector)
    .catch(() => []);
}

/** 无头 Chrome 的经典痕迹：没有真实窗口，outerWidth / outerHeight 为 0 */
function detectZeroOuterSize(): string[] {
  const { outerWidth, outerHeight } = window;
  if (typeof outerWidth === 'number' && typeof outerHeight === 'number') {
    if (outerWidth === 0 || outerHeight === 0) {
      return ['zero_outer_size'];
    }
  }
  return [];
}

/** 桌面 Chrome 必有 window.chrome 对象，缺失说明是精简内核或被篡改的环境 */
function detectChromeObjectMissing(): string[] {
  const ua = navigator.userAgent || '';
  const desktopChrome = /Chrome\//.test(ua) && !/Mobile|Android|iPhone|iPad/.test(ua);
  if (desktopChrome && !('chrome' in window)) {
    return ['chrome_object_missing'];
  }
  return [];
}

/**
 * navigator 关键属性的 getter 是否还是原生实现。
 * stealth 类补丁通常用 defineProperty 覆盖 Navigator.prototype 上的 getter，或直接在 navigator 实例上定义同名属性，
 * 两种方式都会让 Function.prototype.toString 不再返回 [native code]，或让实例多出自有属性。
 */
function detectGetterPatched(): string[] {
  const found: string[] = [];
  const NavigatorCtor = (window as unknown as { Navigator?: { prototype: object } }).Navigator;
  const proto = NavigatorCtor?.prototype;
  for (const prop of PATCHED_GETTER_PROPS) {
    let patched = false;
    if (proto) {
      const getter = Object.getOwnPropertyDescriptor(proto, prop)?.get;
      if (getter && !NATIVE_CODE_PATTERN.test(Function.prototype.toString.call(getter))) {
        patched = true;
      }
    }
    if (Object.getOwnPropertyDescriptor(navigator, prop) !== undefined) {
      patched = true;
    }
    if (patched) {
      found.push(`getter_patched:${prop}`);
    }
  }
  return found;
}

/** 无头 Chrome 旧版本的痕迹：Notification.permission 为 default，但 permissions API 说已被拒绝 */
async function detectPermissionMismatch(): Promise<string[]> {
  const permissions = navigator.permissions;
  const NotificationCtor = (window as unknown as { Notification?: { permission?: string } })
    .Notification;
  if (
    !permissions ||
    typeof permissions.query !== 'function' ||
    !NotificationCtor ||
    typeof NotificationCtor.permission !== 'string'
  ) {
    return [];
  }
  const status = await permissions.query({ name: 'notifications' as PermissionName });
  if (NotificationCtor.permission === 'default' && status.state === 'denied') {
    return ['permission_mismatch'];
  }
  return [];
}

/** 真实设备几乎都有至少一个音视频设备；枚举不需要权限，未授权时只是标签为空 */
async function detectNoMediaDevices(): Promise<string[]> {
  const media = navigator.mediaDevices;
  if (!media || typeof media.enumerateDevices !== 'function') {
    return [];
  }
  const devices = await media.enumerateDevices();
  return devices.length === 0 ? ['no_media_devices'] : [];
}

interface WorkerReport {
  ua?: string;
  cores?: number;
  platform?: string;
}

/**
 * Worker 线程里的 navigator 与主线程比对：只在主线程打补丁的伪装，在 Worker 里会露出真实值。
 * CSP 禁止 blob: worker、构造失败或超时都视为"无信号"。
 */
async function detectWorkerMismatch(timeoutMs: number): Promise<string[]> {
  if (
    typeof Worker === 'undefined' ||
    typeof Blob === 'undefined' ||
    typeof URL === 'undefined' ||
    typeof URL.createObjectURL !== 'function'
  ) {
    return [];
  }
  const source =
    'self.postMessage({ua: navigator.userAgent, cores: navigator.hardwareConcurrency, platform: navigator.platform});';
  let url: string | null = null;
  let worker: Worker | null = null;
  try {
    url = URL.createObjectURL(new Blob([source], { type: 'application/javascript' }));
    const created = new Worker(url);
    worker = created;
    const report = await withTimeout(
      new Promise<WorkerReport | null>((resolve, reject) => {
        created.onmessage = (event: MessageEvent<WorkerReport>) => resolve(event.data);
        created.onerror = () => reject(new Error('worker failed'));
      }),
      timeoutMs,
      null,
    );
    if (!report) {
      return [];
    }
    const found: string[] = [];
    if (typeof report.ua === 'string' && report.ua !== navigator.userAgent) {
      found.push('worker_ua_mismatch');
    }
    if (
      typeof report.cores === 'number' &&
      typeof navigator.hardwareConcurrency === 'number' &&
      report.cores !== navigator.hardwareConcurrency
    ) {
      found.push('worker_cores_mismatch');
    }
    if (
      typeof report.platform === 'string' &&
      typeof navigator.platform === 'string' &&
      navigator.platform !== '' &&
      report.platform !== navigator.platform
    ) {
      found.push('worker_platform_mismatch');
    }
    return found;
  } catch {
    return [];
  } finally {
    worker?.terminate();
    if (url) {
      URL.revokeObjectURL(url);
    }
  }
}

function normalize(markers: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const marker of markers) {
    const trimmed = marker.slice(0, MAX_MARKER_LENGTH);
    if (trimmed && !seen.has(trimmed)) {
      seen.add(trimmed);
      out.push(trimmed);
      if (out.length >= MAX_ANOMALIES) {
        break;
      }
    }
  }
  return out;
}

/**
 * 并行执行全部检测，在 budgetMs 内返回已完成的结果；结果在页面生命周期内缓存。
 * 浏览器之外（SSR、单测的 node 环境未 stub 时）返回空数组。
 */
export async function collectAnomalies(budgetMs = 150): Promise<string[]> {
  if (typeof window === 'undefined' || typeof navigator === 'undefined') {
    return [];
  }
  if (cachedAnomalies) {
    return cachedAnomalies;
  }

  const detectors: Promise<string[]>[] = [
    run(detectZeroOuterSize),
    run(detectChromeObjectMissing),
    run(detectGetterPatched),
    run(detectPermissionMismatch),
    run(detectNoMediaDevices),
    run(() => detectWorkerMismatch(Math.min(WORKER_TIMEOUT_MS, budgetMs))),
  ];

  // 逐项记录已完成的结果：总预算到期时丢掉还没回来的检测，而不是整体放弃
  const settled: string[][] = detectors.map(() => []);
  detectors.forEach((detector, index) => {
    detector.then((markers) => {
      settled[index] = markers;
    });
  });
  await withTimeout(Promise.all(detectors), budgetMs, undefined);

  const anomalies = normalize(settled.flat());
  cachedAnomalies = anomalies;
  return anomalies;
}

/** 仅供测试：清空缓存 */
export function resetAnomaliesCache(): void {
  cachedAnomalies = null;
}
