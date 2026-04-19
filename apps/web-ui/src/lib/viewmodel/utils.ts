/**
 * ViewModel 工具函数
 */

/**
 * 原始类型
 */
type Primitive = string | number | boolean | bigint | symbol | null | undefined;

/**
 * 深度可变类型
 * 移除所有 readonly 修饰符
 */
export type DeepMutable<T> = T extends Primitive
  ? T
  : T extends readonly (infer U)[]
    ? DeepMutable<U>[]
    : T extends object
      ? { -readonly [K in keyof T]: DeepMutable<T[K]> }
      : T;

/**
 * 将 useSnapshot 返回的只读代理对象转换为普通的可变对象
 *
 * 使用场景：
 * - 需要修改 snapshot 数据时
 * - 需要将数据传递给不支持 Proxy 的第三方库时
 * - 需要序列化数据时（如发送到 API）
 *
 * @param obj - useSnapshot 返回的只读对象
 * @returns 深度克隆的可变对象
 *
 * @example
 * const snapshot = viewModel.$useSnapshot()
 * const mutableData = toMutable(snapshot)
 * mutableData.name = 'new name' // 可以修改
 *
 * @example
 * // 发送到 API
 * const snapshot = viewModel.$useSnapshot()
 * await api.post('/user', toMutable(snapshot))
 */
export function toMutable<T>(obj: T): DeepMutable<T> {
  return structuredClone(obj) as DeepMutable<T>;
}
