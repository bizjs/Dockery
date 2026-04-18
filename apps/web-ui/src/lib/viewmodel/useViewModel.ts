/**
 * React Hook for ViewModel
 * 简化的 API：useViewModel 返回 viewModel 实例
 * React 19 兼容：使用 useState 避免在 render 阶段读取 ref
 */

import { useEffect, useRef, useState } from 'react';
import type { BaseViewModel, ViewModelLifecycle } from './BaseViewModel';

/**
 * ViewModel 构造函数类型
 */
type ViewModelConstructor<T extends BaseViewModel> = new (...args: unknown[]) => T;

/**
 * ViewModel 使用选项
 */
export interface UseViewModelOptions {
  /**
   * 组件卸载时是否销毁 ViewModel
   * 默认：传入构造函数时为 true，传入实例时为 false
   */
  destroyOnUnmount?: boolean;
}

/**
 * 使用 ViewModel 的 Hook
 *
 * @example
 * // 用法 1: 传入构造函数，创建局部 ViewModel
 * const viewModel = useViewModel(CounterViewModel)
 * viewModel.increment() // 调用 action
 * const snapshot = viewModel.$useSnapshot() // 订阅状态
 * const count = snapshot.count
 *
 * @example
 * // 用法 2: 传入实例，共享全局 ViewModel
 * const viewModel = useViewModel(counterViewModel)
 * viewModel.increment()
 * const snapshot = viewModel.$useSnapshot()
 */
export function useViewModel<T extends BaseViewModel>(
  viewModelOrCtor: T | ViewModelConstructor<T>,
  options?: UseViewModelOptions,
): T {
  const isConstructor = typeof viewModelOrCtor === 'function';

  // 确定默认的销毁行为
  const destroyOnUnmount = options?.destroyOnUnmount ?? isConstructor;

  // 使用 useState 创建或获取 ViewModel 实例
  // useState 的初始化函数只会在首次渲染时执行一次，保证单例
  const [viewModel] = useState<T>(() => {
    if (isConstructor) {
      return new (viewModelOrCtor as ViewModelConstructor<T>)();
    } else {
      return viewModelOrCtor as T;
    }
  });

  // 使用 useRef 跟踪挂载计数，避免修改 useState 返回的值
  const mountedCountRef = useRef(0);

  // 组件挂载时初始化 ViewModel
  useEffect(() => {
    const vmLife = viewModel as unknown as ViewModelLifecycle & {
      $destroy?: () => Promise<void>;
    };

    // 增加挂载计数
    mountedCountRef.current++;
    if (mountedCountRef.current === 1) {
      vmLife.$onMounted?.();
      console.log('ViewModel mounted');
    }

    // 组件卸载时销毁 ViewModel
    return () => {
      mountedCountRef.current--;

      if (mountedCountRef.current === 0) {
        console.log('ViewModel unmounted');
        vmLife.$onUnMounted?.();
        if (destroyOnUnmount) {
          vmLife.$destroy?.();
        }
      }
    };
  }, [viewModel, destroyOnUnmount]);

  return viewModel;
}
