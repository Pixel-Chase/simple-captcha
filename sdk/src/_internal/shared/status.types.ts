/** 校验结果态：idle 为正常交互，success / fail 为校验返回后短暂展示的结果画面 */
export type CaptchaStatus = 'idle' | 'success' | 'fail';

export interface StatusProps {
  status?: CaptchaStatus;
  /** 从题目呈现到提交答案的耗时（秒），仅 success 态展示 */
  elapsed?: number;
}
