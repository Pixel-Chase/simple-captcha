import type { CaptchaPowChallenge, CaptchaPowSolution } from '../../Captcha.types';
import { encodeUtf8, leadingZeroBits, sha256 } from './sha256';

/** 每计算多少次哈希让出一次主线程，避免阻塞 UI */
const CHUNK_SIZE = 1024;

const yieldToMain = () => new Promise<void>((resolve) => {
  setTimeout(resolve, 0);
});

export interface SolvePowOptions {
  /** 最大尝试次数，默认为 max(2^20, 2^(difficulty + 4)) */
  maxIterations?: number;
  /** 置为 true 时提前终止 */
  cancelled?: { current: boolean };
}

/**
 * 求解工作量证明：寻找 solution 使得 SHA-256(nonce ":" key ":" solution) 的前导零位数 ≥ difficulty。
 * 哈希输入包含题目 key，解答只能在拿到题目之后开始计算，无法预先批量生成。
 * 在题目加载后立即在后台计算，用户完成操作时通常已经算完，不会感知到额外等待。
 */
export async function solvePow(
  challenge: CaptchaPowChallenge,
  key: string,
  options: SolvePowOptions = {},
): Promise<CaptchaPowSolution | null> {
  const difficulty = Math.max(0, Math.floor(challenge.difficulty || 0));
  if (difficulty === 0) {
    return { nonce: challenge.nonce, solution: '0' };
  }

  const limit = options.maxIterations ?? Math.max(1 << 20, 2 ** (difficulty + 4));
  const prefix = encodeUtf8(`${challenge.nonce}:${key}:`);
  const buffer = new Uint8Array(prefix.length + 24);
  buffer.set(prefix);

  for (let counter = 0; counter < limit; counter++) {
    if (options.cancelled?.current) {
      return null;
    }

    const digits = String(counter);
    for (let i = 0; i < digits.length; i++) {
      buffer[prefix.length + i] = digits.charCodeAt(i);
    }

    const digest = sha256(buffer.subarray(0, prefix.length + digits.length));
    if (leadingZeroBits(digest) >= difficulty) {
      return { nonce: challenge.nonce, solution: digits };
    }

    if (counter % CHUNK_SIZE === CHUNK_SIZE - 1) {
      await yieldToMain();
    }
  }

  return null;
}

/** 校验一个解答是否满足难度（供测试与调试使用） */
export function verifyPow(challenge: CaptchaPowChallenge, key: string, solution: string): boolean {
  const digest = sha256(encodeUtf8(`${challenge.nonce}:${key}:${solution}`));
  return leadingZeroBits(digest) >= Math.max(0, challenge.difficulty || 0);
}
