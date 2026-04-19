/**
 * Valtio OOP ViewModel Framework
 * MVVM 模式的 ViewModel 层实现
 * 导出所有公共 API
 */

// 基础类和类型
export { BaseViewModel, createViewModel } from './BaseViewModel';
export type { ViewModelLifecycle, ViewModelOptions } from './BaseViewModel';

// React Hook
export { useViewModel } from './useViewModel';
export type { UseViewModelOptions } from './useViewModel';

// 工具函数
export { toMutable } from './utils';
export type { DeepMutable } from './utils';
