# Valtio OOP ViewModel 框架

基于 Valtio 的面向对象状态管理框架，提供生命周期函数和类型安全的状态管理能力。

## 特性

- ✅ **OOP 模式**：使用类来组织状态和逻辑
- ✅ **生命周期函数**：支持 `$onInit`、`$onMounted`、`$onDestroy`、`$onStateChange`、`$onError`
- ✅ **类型安全**：完整的 TypeScript 类型支持
- ✅ **响应式**：基于 Valtio 的响应式状态管理
- ✅ **计算属性**：支持 getter 作为计算属性
- ✅ **Watch API**：类似 Vue 的 `$watch` 监听属性变化
- ✅ **React 集成**：提供 React Hooks

## 快速开始

### 1. 创建 ViewModel

```typescript
import { BaseViewModel } from '@/lib/viewmodel/BaseViewModel';

interface UserState {
  name: string;
  age: number;
  isLoggedIn: boolean;
}

export class UserViewModel extends BaseViewModel<UserState> {
  constructor() {
    super(
      {
        name: '',
        age: 0,
        isLoggedIn: false,
      },
      {
        enableStateChangeListener: true,
      },
    );
  }

  // 生命周期：初始化（ViewModel 创建时调用）
  async $onInit() {
    console.log('UserViewModel initialized');
    // ViewModel 初始化工作，如设置默认值
  }

  // 生命周期：组件挂载（React 组件挂载时调用）
  async $onMounted() {
    console.log('UserViewModel mounted');
    // 组件挂载后的工作，如加载数据、订阅事件
    await this.loadUser();
  }

  // 生命周期：销毁（组件卸载时调用）
  async $onDestroy() {
    console.log('UserViewModel destroyed');
    // 清理资源、取消订阅
  }

  // 生命周期：状态变化
  $onStateChange(snapshot: UserState) {
    console.log('State changed:', snapshot);
    // 可以在这里持久化状态
  }

  // 生命周期：错误处理
  $onError(error: Error) {
    console.error('UserViewModel error:', error);
    // 统一错误处理
  }

  // Actions
  async loadUser() {
    try {
      const response = await fetch('/api/user');
      const data = await response.json();
      this.$updateState({
        name: data.name,
        age: data.age,
        isLoggedIn: true,
      });
    } catch (error) {
      this.$onError?.(error as Error);
    }
  }

  login(name: string) {
    this.$updateState({
      name,
      isLoggedIn: true,
    });
  }

  logout() {
    this.$updateState({
      name: '',
      age: 0,
      isLoggedIn: false,
    });
  }

  // 计算属性
  get isAdult(): boolean {
    return this.state.age >= 18;
  }

  get displayName(): string {
    return this.state.isLoggedIn ? this.state.name : 'Guest';
  }
}

// 创建单例实例
export const userViewModel = new UserViewModel();
```

### 2. 在 React 组件中使用

```typescript
import { useViewModel } from '@/lib/viewmodel/useViewModel'
import { userViewModel, UserViewModel } from './UserViewModel'

function UserProfile() {
  // 方式 1: 使用全局单例 ViewModel
  const viewModel = useViewModel(userViewModel)
  const state = viewModel.$useSnapshot()

  return (
    <div>
      <h1>Welcome, {state.name}!</h1>
      <p>Age: {state.age}</p>
      <p>Status: {state.isLoggedIn ? 'Logged in' : 'Logged out'}</p>
      <p>Is Adult: {viewModel.isAdult ? 'Yes' : 'No'}</p>

      <button onClick={() => viewModel.login('John')}>Login</button>
      <button onClick={() => viewModel.logout()}>Logout</button>
    </div>
  )
}

function UserActions() {
  // 方式 2: 创建局部 ViewModel（每个组件实例独立）
  const viewModel = useViewModel(UserViewModel)
  const state = viewModel.$useSnapshot()

  return (
    <div>
      <p>Count: {state.name}</p>
      <button onClick={() => viewModel.login('Alice')}>Login as Alice</button>
      <button onClick={() => viewModel.logout()}>Logout</button>
    </div>
  )
}
```

### 3. 不使用 React Hook 的方式

```typescript
import { userViewModel } from './UserViewModel';

// 直接使用 ViewModel 实例
userViewModel.login('John');

// 获取状态快照
const snapshot = userViewModel.$getSnapshot();
console.log(snapshot.name);

// 订阅状态变化
const unsubscribe = userViewModel.$subscribe((snapshot) => {
  console.log('State changed:', snapshot);
});

// 取消订阅
unsubscribe();
```

## API 参考

### BaseViewModel

#### 构造函数

```typescript
constructor(initialState: T, options?: ViewModelOptions)
```

**参数：**

- `initialState`: 初始状态对象
- `options`: 配置选项
  - `enableStateChangeListener`: 是否启用状态变化监听（默认：false）

#### 生命周期方法

```typescript
// 初始化时调用（ViewModel 创建时）
async $onInit?(): void | Promise<void>

// 组件挂载时调用（React 组件挂载时）
async $onMounted?(): void | Promise<void>

// 销毁时调用（组件卸载时）
async $onDestroy?(): void | Promise<void>

// 状态变化时调用
$onStateChange?(snapshot: T): void

// 错误处理
$onError?(error: Error): void
```

**生命周期执行顺序：**

1. `constructor` - ViewModel 实例创建
2. `$onInit` - ViewModel 初始化
3. `$onMounted` - React 组件挂载时（在 `useViewModel` Hook 中）
4. `$onStateChange` - 状态变化时（如果 `enableStateChangeListener: true`）
5. `$onDestroy` - 组件卸载时（如果配置了销毁）

**使用场景：**

- `$onInit`: ViewModel 级别的初始化，如设置默认配置
- `$onMounted`: 组件级别的初始化，如加载数据、订阅事件
- `$onDestroy`: 清理资源、取消订阅
- `$onStateChange`: 状态持久化、日志记录
- `$onError`: 统一错误处理

#### 实例方法

```typescript
// 初始化 ViewModel
protected async $init(): Promise<void>

// 销毁 ViewModel
protected async $destroy(): Promise<void>

// 获取状态快照（不可变）
$getSnapshot(): T

// 获取代理状态（可变）
$getState(): T

// React Hook: 订阅状态变化
$useSnapshot(): T

// 批量更新状态
protected $updateState(updates: Partial<T>): void

// 订阅状态变化
protected $subscribe(callback: (snapshot: T) => void): () => void

// 监听特定属性变化（类似 Vue 的 watch）
protected $watch<K extends keyof T>(
  key: K,
  callback: (value: T[K]) => void,
  options?: { immediate?: boolean }
): () => void

// 监听多个属性变化
protected $watchMultiple<K extends keyof T>(
  keys: K[],
  callback: (snapshot: Pick<T, K>) => void
): () => void
```

**$watch 示例：**

```typescript
class UserViewModel extends BaseViewModel<UserState> {
  async $onMounted() {
    // 监听单个属性
    const unwatch = this.$watch('name', (newName) => {
      console.log('Name changed to:', newName);
    });

    // 立即执行一次
    this.$watch(
      'age',
      (age) => {
        console.log('Current age:', age);
      },
      { immediate: true },
    );

    // 监听多个属性
    this.$watchMultiple(['name', 'age'], (snapshot) => {
      console.log('Name or age changed:', snapshot);
    });

    // 取消监听
    // unwatch()
  }
}
```

#### 实例属性

```typescript
// 是否已初始化
get initialized(): boolean

// 是否已销毁
get destroyed(): boolean

// 访问状态（仅供子类使用）
protected get state(): T
```

### React Hooks

#### useViewModel

使用 ViewModel 的 Hook，支持传入构造函数或实例。

```typescript
function useViewModel<T extends BaseViewModel>(
  viewModelOrCtor: T | ViewModelConstructor<T>,
  options?: UseViewModelOptions,
): T;
```

**参数：**

- `viewModelOrCtor`: ViewModel 实例或构造函数
- `options`: 配置选项
  - `destroyOnUnmount`: 组件卸载时是否销毁 ViewModel（默认：传入构造函数时为 true，传入实例时为 false）

**示例：**

```typescript
// 用法 1: 传入构造函数，创建局部 ViewModel
const viewModel = useViewModel(CounterViewModel);

// 用法 2: 传入实例，共享全局 ViewModel
const viewModel = useViewModel(counterViewModel);

// 用法 3: 自定义销毁行为
const viewModel = useViewModel(counterViewModel, { destroyOnUnmount: true });
```

### 工具函数

#### createViewModel

创建 ViewModel 的工厂函数。

```typescript
function createViewModel<T extends BaseViewModel>(ViewModelClass: new (...args: unknown[]) => T, ...args: unknown[]): T;
```

#### toMutable

将 `$useSnapshot` 返回的只读代理对象转换为普通的可变对象。

```typescript
function toMutable<T>(obj: T): DeepMutable<T>;
```

**使用场景：**

- 需要修改 snapshot 数据时
- 需要将数据传递给不支持 Proxy 的第三方库时
- 需要序列化数据时（如发送到 API）

**示例：**

```typescript
const snapshot = viewModel.$useSnapshot();
const mutableData = toMutable(snapshot);
mutableData.name = 'new name'; // 可以修改

// 发送到 API
await api.post('/user', toMutable(snapshot));
```

## 最佳实践

### 1. 状态设计

```typescript
// ✅ 好的做法：扁平化状态
interface GoodState {
  userId: string;
  userName: string;
  userAge: number;
}

// ❌ 避免：过度嵌套
interface BadState {
  user: {
    profile: {
      info: {
        name: string;
      };
    };
  };
}
```

### 2. Action 方法

```typescript
// ✅ 好的做法：使用描述性的方法名
class UserViewModel extends BaseViewModel<UserState> {
  async loadUserProfile() {}
  updateUserName(name: string) {}
  deleteUserAccount() {}
}

// ❌ 避免：通用的方法名
class UserViewModel extends BaseViewModel<UserState> {
  async load() {}
  update(data: any) {}
  delete() {}
}
```

### 3. 错误处理

```typescript
class UserViewModel extends BaseViewModel<UserState> {
  $onError(error: Error) {
    // 统一错误处理
    console.error('UserViewModel error:', error);
    // 可以显示错误提示
    toast.error(error.message);
  }

  async loadUser() {
    try {
      // API 调用
    } catch (error) {
      this.$onError?.(error as Error);
      throw error; // 重新抛出以便调用者处理
    }
  }
}
```

### 4. 计算属性

```typescript
class UserViewModel extends BaseViewModel<UserState> {
  // ✅ 使用 getter 作为计算属性
  get fullName(): string {
    return `${this.state.firstName} ${this.state.lastName}`;
  }

  get isVip(): boolean {
    return this.state.points > 1000;
  }
}
```

### 5. 状态持久化

```typescript
class UserViewModel extends BaseViewModel<UserState> {
  constructor() {
    super(initialState, {
      enableStateChangeListener: true, // 启用状态变化监听
    });
  }

  $onStateChange(snapshot: UserState) {
    // 持久化到 localStorage
    localStorage.setItem('user', JSON.stringify(snapshot));
  }

  async $onInit() {
    // 从 localStorage 恢复状态
    const saved = localStorage.getItem('user');
    if (saved) {
      this.$updateState(JSON.parse(saved));
    }
  }
}
```

### 6. 使用 $watch 监听属性变化

```typescript
class CounterViewModel extends BaseViewModel<CounterState> {
  async $onMounted() {
    // 监听 count 变化
    this.$watch('count', (newValue) => {
      console.log('Count changed to:', newValue);

      // 当 count 达到 10 时，重置
      if (newValue >= 10) {
        this.reset();
      }
    });

    // 监听多个属性
    this.$watchMultiple(['count', 'step'], (snapshot) => {
      console.log('Count or step changed:', snapshot);
    });
  }
}
```

## 示例

查看 `examples/CounterViewModel.ts` 获取完整示例。

## 注意事项

1. **单例 vs 多例**：
   - 全局共享的状态：创建单例实例，传入 `useViewModel(viewModelInstance)`
   - 组件级别的状态：传入构造函数，`useViewModel(ViewModelClass)`

2. **生命周期**：
   - `$onInit` 在构造函数之后自动调用
   - `$onMounted` 在 React 组件挂载时调用
   - `$onDestroy` 在组件卸载时调用（如果配置了销毁）

3. **状态更新**：
   - 使用 `this.$updateState()` 批量更新状态
   - 或直接修改 `this.state.xxx`（Valtio 会自动追踪）
   - 不要替换整个 state 对象

4. **计算属性**：
   - getter 会在每次访问时重新计算
   - 注意性能，避免在 getter 中进行复杂计算

5. **错误处理**：
   - 在 `$onError` 中统一处理错误
   - 避免在每个 action 中重复处理

6. **方法绑定**：
   - 框架会自动绑定所有方法的 `this`
   - 可以安全地将方法作为回调传递：`<button onClick={viewModel.increment} />`

## License

MIT
