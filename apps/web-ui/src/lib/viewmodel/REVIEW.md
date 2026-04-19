# ViewModel Framework Code Review

## 概述

这是一个基于 Valtio 的 MVVM 模式 ViewModel 框架，提供了面向对象的状态管理能力。整体设计清晰，实现简洁，是一个实用性强的状态管理方案。

---

## 🎯 优点

1. **清晰的 MVVM 架构**：职责分离明确，View 和 Model 通过 ViewModel 连接
2. **完整的生命周期管理**：提供 `$onInit`、`$onMounted`、`$onDestroy`、`$onStateChange`、`$onError` 钩子
3. **响应式状态管理**：基于 Valtio 的 proxy 机制，自动追踪状态变化
4. **类型安全**：完整的 TypeScript 类型支持
5. **Watch API**：提供类似 Vue 的 `$watch` 和 `$watchMultiple` API
6. **自动方法绑定**：自动绑定类方法的 `this` 上下文，简化事件处理
7. **简洁的 API 设计**：`$useSnapshot()` 提供便捷的状态订阅方式
8. **正确的 Hook 实现**：`useViewModel` 使用 `useRef` 确保实例只创建一次

---

## 🔴 需要修复的问题

### 1. **__bindMethods 中的逻辑错误**

**问题代码：**
```typescript
private __bindMethods(): void {
  const prototype = Object.getPrototypeOf(this);
  const propertyNames = Object.getOwnPropertyNames(prototype);

  const self = this as Record<string, unknown>;

  propertyNames.forEach((name) => {
    // 🔴 BUG: 这个条件永远不会跳过任何方法！
    if ((name === 'constructor' || name.startsWith('__')) || name.startsWith('$')) {
      return;
    }
    // ...
  });
}
```

**问题分析：**
- `(name === 'constructor' && name.startsWith('__'))` 这个条件永远为 false
  - 因为 `'constructor'` 不以 `'__'` 开头
- 应该使用 `||` 而不是 `&&`

**修复方案：**
```typescript
private __bindMethods(): void {
  const prototype = Object.getPrototypeOf(this);
  const propertyNames = Object.getOwnPropertyNames(prototype);

  const self = this as Record<string, unknown>;

  propertyNames.forEach((name) => {
    // ✅ 修复：使用 || 连接条件
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
```

### 2. **useViewModel 中的生命周期方法调用错误**

**问题代码：**
```typescript
export function useViewModel<T extends BaseViewModel>(
  viewModelOrCtor: T | ViewModelConstructor<T>,
  options?: UseViewModelOptions
): T {
  // ...
  
  useEffect(() => {
    if (!viewModel.initialized) {
      viewModel.$init()  // ✅ 正确
    }

    // 🔴 错误：应该是 $onMounted 而不是 onMounted
    const mountedPromise = viewModel.onMounted?.()
    if (mountedPromise instanceof Promise) {
      mountedPromise.catch((error) => {
        console.error(`Error in ${viewModel.constructor.name}.onMounted:`, error)
      })
    }

    return () => {
      if (destroyOnUnmount && !viewModel.destroyed) {
        viewModel.$destroy()  // ✅ 正确
      }
    }
  }, [viewModel, destroyOnUnmount])

  return viewModel
}
```

**问题：**
- 应该调用 `viewModel.$onMounted?.()` 而不是 `viewModel.onMounted?.()`
- 生命周期方法都使用 `$on` 前缀

**修复方案：**
```typescript
useEffect(() => {
  if (!viewModel.initialized) {
    viewModel.$init()
  }

  // ✅ 修复：调用 $onMounted
  const mountedPromise = viewModel.$onMounted?.()
  if (mountedPromise instanceof Promise) {
    mountedPromise.catch((error) => {
      console.error(`Error in ${viewModel.constructor.name}.$onMounted:`, error)
    })
  }

  return () => {
    if (destroyOnUnmount && !viewModel.destroyed) {
      viewModel.$destroy()
    }
  }
}, [viewModel, destroyOnUnmount])
```

### 3. **$init 和 $destroy 的访问控制不一致**

**问题：**
```typescript
// BaseViewModel.ts
protected async $init(): Promise<void> { }  // protected
protected async $destroy(): Promise<void> { }  // protected

// useViewModel.ts 中被外部调用
viewModel.$init()    // TypeScript 会报错
viewModel.$destroy() // TypeScript 会报错
```

**修复方案：**
```typescript
// 改为 public，允许外部调用
public async $init(): Promise<void> {
  if (this._initialized) {
    throw new Error(`${this.constructor.name} is already initialized`);
  }

  try {
    await this.$onInit?.();
    this._initialized = true;
  } catch (error) {
    this.__handleError(error as Error);
  }
}

public async $destroy(): Promise<void> {
  if (this._destroyed) {
    console.warn(`${this.constructor.name} already destroyed`);
    return;
  }

  try {
    await this.$onDestroy?.();
    this.unsubscribe?.();
    this._destroyed = true;
  } catch (error) {
    this.__handleError(error as Error);
  }
}
```

---

## 🟡 建议改进的问题

### 4. **createViewModel 函数的注释与实现不符**

**问题：**
```typescript
/**
 * 创建 ViewModel 的工厂函数
 * 注意：如果 ViewModel 构造函数中设置了 autoInit: false，则不会自动调用 $init
 */
export function createViewModel<T extends BaseViewModel>(
  ViewModelClass: new (...args: unknown[]) => T,
  ...args: unknown[]
): T {
  const instance = new ViewModelClass(...args);
  if (!instance.initialized) {
    // force call $init()
    (instance as unknown as { $init: () => Promise<void> }).$init();
  }

  return instance;
}
```

**问题分析：**
1. 注释说"不会自动调用 $init"，但代码实际上会强制调用
2. `ViewModelOptions` 中已经没有 `autoInit` 选项了
3. 构造函数中已经自动调用 `this.$init()`

**建议方案：**

```typescript
// 方案 1：简化为直接返回实例（推荐）
export function createViewModel<T extends BaseViewModel>(
  ViewModelClass: new (...args: unknown[]) => T,
  ...args: unknown[]
): T {
  return new ViewModelClass(...args);
}

// 方案 2：移除这个函数，直接使用 new
// const viewModel = new MyViewModel();
```

### 5. **toMutable 函数缺少兼容性处理**

**当前实现：**
```typescript
export function toMutable<T>(obj: T): DeepMutable<T> {
  return structuredClone(obj) as DeepMutable<T>;
}
```

**问题：**
- `structuredClone` 是较新的 API（Node 17+, Chrome 98+）
- 在某些环境中可能不支持
- 没有错误处理

**建议改进：**
```typescript
export function toMutable<T>(obj: T): DeepMutable<T> {
  // 检查是否支持 structuredClone
  if (typeof structuredClone !== 'undefined') {
    try {
      return structuredClone(obj) as DeepMutable<T>;
    } catch (error) {
      console.warn('structuredClone failed, falling back to JSON clone', error);
    }
  }
  
  // 降级方案：使用 JSON（注意：无法克隆函数、Symbol、undefined 等）
  try {
    return JSON.parse(JSON.stringify(obj)) as DeepMutable<T>;
  } catch (error) {
    console.error('Failed to clone object', error);
    throw error;
  }
}
```

### 6. **错误处理可以更健壮**

**当前实现：**
```typescript
protected async $init(): Promise<void> {
  if (this._initialized) {
    throw new Error(`${this.constructor.name} is already initialized`);
  }

  try {
    await this.$onInit?.();
    this._initialized = true;
  } catch (error) {
    this.__handleError(error as Error);
    // 🔴 问题：错误被吞掉了，调用者不知道初始化失败
  }
}
```

**建议改进：**
```typescript
protected async $init(): Promise<void> {
  if (this._initialized) {
    throw new Error(`${this.constructor.name} is already initialized`);
  }

  try {
    await this.$onInit?.();
    this._initialized = true;
  } catch (error) {
    this.__handleError(error as Error);
    throw error; // 重新抛出，让调用者知道初始化失败
  }
}
```

### 7. **缺少单元测试**

**问题：**
- 没有任何单元测试或集成测试
- 无法保证代码质量和功能正确性
- 重构时容易引入 bug（如上面发现的 `__bindMethods` bug）

**建议：**
创建测试文件：

```typescript
// __tests__/lib/viewmodel/BaseViewModel.test.ts
import { describe, it, expect, vi } from 'vitest';
import { BaseViewModel } from '../BaseViewModel';

describe('BaseViewModel', () => {
  it('should initialize correctly', async () => {
    class TestViewModel extends BaseViewModel<{ count: number }> {
      constructor() {
        super({ count: 0 });
      }
    }
    
    const vm = new TestViewModel();
    expect(vm.initialized).toBe(true);
  });

  it('should call lifecycle hooks', async () => {
    const onInit = vi.fn();
    
    class TestViewModel extends BaseViewModel<{ count: number }> {
      constructor() {
        super({ count: 0 });
      }
      async $onInit() {
        onInit();
      }
    }
    
    const vm = new TestViewModel();
    expect(onInit).toHaveBeenCalled();
  });

  it('should update state correctly', () => {
    class TestViewModel extends BaseViewModel<{ count: number }> {
      constructor() {
        super({ count: 0 });
      }
      increment() {
        this.state.count++;
      }
    }
    
    const vm = new TestViewModel();
    vm.increment();
    expect(vm.$getSnapshot().count).toBe(1);
  });

  it('should bind methods correctly', () => {
    class TestViewModel extends BaseViewModel<{ count: number }> {
      constructor() {
        super({ count: 0 });
      }
      increment() {
        this.state.count++;
      }
    }
    
    const vm = new TestViewModel();
    const { increment } = vm;
    increment(); // 应该不会报错
    expect(vm.$getSnapshot().count).toBe(1);
  });

  it('should watch property changes', async () => {
    const callback = vi.fn();
    
    class TestViewModel extends BaseViewModel<{ count: number }> {
      constructor() {
        super({ count: 0 });
      }
      async $onMounted() {
        this.$watch('count', callback);
      }
    }
    
    const vm = new TestViewModel();
    await vm.$onMounted?.();
    vm.state.count = 1;
    
    // 等待微任务
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(callback).toHaveBeenCalledWith(1);
  });
});

// __tests__/lib/viewmodel/useViewModel.test.tsx
import { renderHook } from '@testing-library/react';
import { useViewModel } from '../useViewModel';
import { BaseViewModel } from '../BaseViewModel';

describe('useViewModel', () => {
  it('should create instance from constructor', () => {
    class TestViewModel extends BaseViewModel<{ count: number }> {
      constructor() {
        super({ count: 0 });
      }
    }
    
    const { result } = renderHook(() => useViewModel(TestViewModel));
    expect(result.current).toBeInstanceOf(TestViewModel);
  });

  it('should use existing instance', () => {
    class TestViewModel extends BaseViewModel<{ count: number }> {
      constructor() {
        super({ count: 0 });
      }
    }
    
    const instance = new TestViewModel();
    const { result } = renderHook(() => useViewModel(instance));
    expect(result.current).toBe(instance);
  });

  it('should call $onMounted on mount', async () => {
    const onMounted = vi.fn();
    
    class TestViewModel extends BaseViewModel<{ count: number }> {
      constructor() {
        super({ count: 0 });
      }
      async $onMounted() {
        onMounted();
      }
    }
    
    renderHook(() => useViewModel(TestViewModel));
    
    // 等待 useEffect
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(onMounted).toHaveBeenCalled();
  });
});
```

---

## 🟢 设计权衡说明

### 8. **$useSnapshot 的设计**

**当前实现：**
```typescript
$useSnapshot(): T {
  // eslint-disable-next-line react-hooks/rules-of-hooks
  return useSnapshot(this._state) as T;
}
```

**设计权衡分析：**

✅ **优点：**
- API 简洁，用户体验好：`viewModel.$useSnapshot()`
- 将 Valtio 的实现细节封装在 ViewModel 内部
- 符合面向对象的封装原则
- 在实际使用中（React 组件内）不会有问题

⚠️ **权衡：**
- 违反了 React Hooks 的规则（Hook 只能在组件顶层调用）
- 需要使用 `eslint-disable` 来绕过检查
- 这是一个有意识的设计决策，优先考虑了 API 的简洁性

**结论：** 这是一个合理的设计权衡。如果在实际使用中没有遇到问题，可以保持现状。这是"约定优于配置"的设计理念。

### 9. **useViewModel 使用 useRef 而不是 useMemo**

**当前实现：**
```typescript
export function useViewModel<T extends BaseViewModel>(
  viewModelOrCtor: T | ViewModelConstructor<T>,
  options?: UseViewModelOptions
): T {
  const viewModelRef = useRef<T | null>(null);
  const isConstructor = typeof viewModelOrCtor === 'function';

  if (viewModelRef.current == null) {
    if (isConstructor) {
      viewModelRef.current = new (viewModelOrCtor as ViewModelConstructor<T>)();
    } else {
      viewModelRef.current = viewModelOrCtor as T;
    }
  }
  // ...
}
```

**为什么不使用 useMemo？**

```typescript
// ❌ 不推荐：useMemo 在 React 18 Strict Mode 下会初始化多次
const viewModel = useMemo(() => {
  if (isConstructor) {
    return new (viewModelOrCtor as ViewModelConstructor<T>)();
  }
  return viewModelOrCtor as T;
}, []);
```

**原因：**
- 在 React 18 的 Strict Mode（开发环境）下，`useMemo` 会被调用两次
- 这会导致 ViewModel 被创建两次，触发两次 `$onInit`
- 对于有副作用的初始化（如 API 调用），这是不可接受的

**当前实现的优势：**
1. ✅ 实例只创建一次，即使在 Strict Mode 下
2. ✅ 不会触发多次初始化
3. ✅ 符合 React 的最佳实践

**结论：** 当前的 `useRef` 实现是正确的，这是最佳方案。

---

## 🎨 可选的架构改进建议

### 10. **考虑添加状态验证钩子**

```typescript
export abstract class BaseViewModel<T extends object = object> {
  // 允许子类自定义状态更新逻辑
  protected beforeStateChange?(updates: Partial<T>): void;
  protected afterStateChange?(snapshot: T): void;
  
  protected $updateState(updates: Partial<T>): void {
    // 验证
    if (!updates || typeof updates !== 'object') {
      throw new Error('Invalid state updates');
    }
    
    // 状态变化前的钩子
    this.beforeStateChange?.(updates);
    
    Object.assign(this._state, updates);
    
    // 状态变化后的钩子
    this.afterStateChange?.(this.$getSnapshot());
  }
}
```

### 11. **考虑添加批量更新 API**

```typescript
export abstract class BaseViewModel<T extends object = object> {
  // 提供批量更新 API，减少重渲染
  protected $batchUpdate(updater: (state: T) => void): void {
    updater(this._state);
  }
}

// 使用
viewModel.$batchUpdate((state) => {
  state.count = 10;
  state.step = 2;
  state.history = [];
});
```

### 12. **考虑添加插件系统**

```typescript
export interface ViewModelPlugin<T> {
  onInit?(state: T): void;
  onStateChange?(state: T): void;
  onDestroy?(): void;
}

export class PersistencePlugin<T> implements ViewModelPlugin<T> {
  constructor(private key: string) {}
  
  onInit(state: T) {
    const saved = localStorage.getItem(this.key);
    if (saved) {
      Object.assign(state, JSON.parse(saved));
    }
  }
  
  onStateChange(state: T) {
    localStorage.setItem(this.key, JSON.stringify(state));
  }
}

// 使用
export interface ViewModelOptions {
  enableStateChangeListener?: boolean;
  plugins?: ViewModelPlugin<any>[];
}
```

---

## 📋 改进建议优先级

### P0 - 必须修复（影响功能正确性）

1. ✅ **修复 `__bindMethods` 中的逻辑错误**
2. ✅ **修复 `useViewModel` 中的 `onMounted` 调用错误**
3. ✅ **修复 `$init` 和 `$destroy` 的访问控制**

### P1 - 建议修复（提升代码质量）

4. ✅ 添加单元测试和集成测试
5. ✅ 改进错误处理（重新抛出错误）
6. ✅ 改进 `toMutable` 的兼容性
7. ✅ 修正 `createViewModel` 的注释或实现

### P2 - 可选改进（提升用户体验）

8. ✅ 添加状态验证钩子
9. ✅ 添加批量更新 API
10. ✅ 考虑添加插件系统
11. ✅ 完善 JSDoc 注释
12. ✅ 添加开发工具支持

---

## 🎯 总结

这个 ViewModel 框架的设计整体上是优秀的：

### ✅ 设计优点
1. **清晰的架构**：MVVM 模式应用得当
2. **完整的功能**：生命周期、Watch API、自动绑定等
3. **正确的 Hook 实现**：使用 `useRef` 避免 Strict Mode 问题
4. **合理的设计权衡**：`$useSnapshot()` 优先考虑 API 简洁性

### 🔴 需要立即修复
1. **`__bindMethods` 逻辑错误**：条件判断有 bug
2. **`useViewModel` 调用错误**：应该调用 `$onMounted` 而不是 `onMounted`
3. **访问控制不一致**：`$init` 和 `$destroy` 应该是 public

### 📈 改进方向
1. **添加测试**：这是最重要的，可以及早发现 bug
2. **改进错误处理**：重新抛出错误，让调用者知道失败
3. **完善文档**：JSDoc 注释、使用指南等

**总体评价：** 这是一个设计良好、实用性强的状态管理框架，修复上述 bug 后将更加完善！

---

## 📚 参考资源

- [Valtio 官方文档](https://github.com/pmndrs/valtio)
- [React Hooks 最佳实践](https://react.dev/reference/react)
- [React 18 Strict Mode](https://react.dev/reference/react/StrictMode)
- [TypeScript 最佳实践](https://www.typescriptlang.org/docs/handbook/declaration-files/do-s-and-don-ts.html)
- [MVVM 模式](https://en.wikipedia.org/wiki/Model%E2%80%93view%E2%80%93viewmodel)
