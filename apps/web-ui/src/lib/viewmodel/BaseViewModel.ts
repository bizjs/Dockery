/**
 * BaseViewModel - Valtio OOP 封装基类
 * MVVM 模式的 ViewModel 层，连接 View 和 Model
 */

import { proxy, subscribe, snapshot, useSnapshot } from 'valtio';
import { subscribeKey } from 'valtio/utils';

/**
 * 生命周期钩子类型
 */
export interface ViewModelLifecycle {
  $onMounted?(): void | Promise<void>;
  $onUnMounted?(): void | Promise<void>;
}

/**
 * ViewModel 配置选项
 */
export interface ViewModelOptions {
  [key: string]: string;
}

/**
 * BaseViewModel 抽象基类
 */
export abstract class BaseViewModel<T extends object = object> {
  public state: T;
  private unsubscribe?: () => void;
  protected options: ViewModelOptions;

  constructor(initialState: T, options: ViewModelOptions = {}) {
    this.options = {
      ...options,
    };

    this.state = proxy(initialState);

    // 自动绑定所有方法的 this
    this.__bindMethods();
  }

  protected async $destroy(): Promise<void> {
    this.unsubscribe?.();
  }

  protected $getSnapshot(): T {
    return snapshot(this.state) as T;
  }

  /**
   * React Hook: 订阅状态变化
   * 用法: const snapshot = viewModel.$useSnapshot()
   */
  $useSnapshot(): T {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    return useSnapshot(this.state) as T;
  }

  protected $updateState(updates: Partial<T>): void {
    Object.assign(this.state, updates);
  }

  protected $subscribe(callback: (snapshot: T) => void): () => void {
    return subscribe(this.state, () => {
      callback(this.$getSnapshot());
    });
  }

  /**
   * 监听特定属性的变化（使用 Valtio 的 subscribeKey）
   * @param key 要监听的属性名
   * @param callback 变化时的回调函数
   * @param options 监听选项
   * @returns 取消监听的函数
   *
   * @example
   * const unwatch = viewModel.$watch('count', (newValue) => {
   *   console.log('count changed to:', newValue)
   * })
   *
   * @example
   * // 立即执行一次
   * const unwatch = viewModel.$watch('count', (newValue) => {
   *   console.log('current count:', newValue)
   * }, { immediate: true })
   *
   * // 取消监听
   * unwatch()
   */
  protected $watch<K extends keyof T>(
    key: K,
    callback: (value: T[K]) => void,
    options?: {
      immediate?: boolean; // 是否立即执行一次回调
    },
  ): () => void {
    // 如果设置了 immediate，立即执行一次
    if (options?.immediate) {
      callback(this.state[key]);
    }

    // 使用 subscribeKey 监听特定属性
    return subscribeKey(this.state, key, callback);
  }

  /**
   * 监听多个属性的变化（批量更新时只触发一次）
   * 使用 subscribeKey + 微任务队列实现
   * @param keys 要监听的属性名数组
   * @param callback 变化时的回调函数
   * @returns 取消监听的函数
   *
   * @example
   * const unwatch = viewModel.$watchMultiple(['count', 'step'], (snapshot) => {
   *   console.log('count or step changed:', snapshot)
   * })
   */
  protected $watchMultiple<K extends keyof T>(keys: K[], callback: (snapshot: Pick<T, K>) => void): () => void {
    let pending = false;
    let cancelled = false;

    const triggerCallback = () => {
      if (pending || cancelled) return;
      pending = true;

      Promise.resolve().then(() => {
        if (cancelled) return; // 检查是否已取消
        pending = false;
        const snapshot = {} as Pick<T, K>;
        keys.forEach((key) => {
          snapshot[key] = this.state[key];
        });
        callback(snapshot);
      });
    };

    const unsubscribes = keys.map((key) => {
      return subscribeKey(this.state, key, triggerCallback);
    });

    return () => {
      cancelled = true; // 标记为已取消
      unsubscribes.forEach((unsubscribe) => unsubscribe());
    };
  }

  /**
   * 自动绑定所有方法的 this
   * 遍历原型链，绑定所有非生命周期、非私有的方法
   */
  private __bindMethods(): void {
    const prototype = Object.getPrototypeOf(this);
    const propertyNames = Object.getOwnPropertyNames(prototype);

    const self = this as Record<string, unknown>;

    propertyNames.forEach((name) => {
      // 跳过特定方法
      if (name === 'constructor' || name.startsWith('__') || name.startsWith('$')) {
        return;
      }
      // 跳过 getter/setter
      const descriptor = Object.getOwnPropertyDescriptor(prototype, name);
      if (descriptor && (descriptor.get || descriptor.set)) {
        return;
      }

      const property = self[name];

      // 只绑定函数
      if (typeof property === 'function') {
        self[name] = property.bind(this);
      }
    });
  }
}

/**
 * 创建 ViewModel 的工厂函数
 * 注意：如果 ViewModel 构造函数中设置了 autoInit: false，则不会自动调用 $init
 */
export function createViewModel<T extends BaseViewModel>(
  ViewModelClass: new (...args: unknown[]) => T,
  ...args: unknown[]
): T {
  const instance = new ViewModelClass(...args) as unknown as { initialized: boolean; $init: () => Promise<void> };
  if (!instance.initialized) {
    // force call $init()
    instance.$init();
  }

  return instance as unknown as T;
}
