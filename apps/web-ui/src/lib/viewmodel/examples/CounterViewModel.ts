/**
 * CounterViewModel - 示例 ViewModel
 * 演示如何使用 BaseViewModel 创建自定义 ViewModel
 */

import { BaseViewModel } from '../BaseViewModel';

/**
 * Counter 状态接口
 */
interface CounterState {
  count: number;
  step: number;
  history: number[];
}

/**
 * CounterViewModel 类
 */
export class CounterViewModel extends BaseViewModel<CounterState> {
  constructor() {
    super(
      {
        count: 0,
        step: 1,
        history: [],
      },
      {},
    );
  }

  /**
   * 生命周期：组件挂载（React 组件挂载时调用）
   */
  async $onMounted() {
    console.log('CounterViewModel mounted');
    // 可以在这里加载数据、订阅事件等
    // 例如：从 API 加载初始数据

    // 示例：监听 count 变化
    this.$watch('count', (newValue) => {
      console.log('Count changed to:', newValue);

      // 当 count 达到 10 时，重置
      if (newValue >= 10) {
        console.log('Count reached 10, resetting...');
        this.reset();
      }
    });
  }

  /**
   * 生命周期：销毁（组件卸载时调用）
   */
  async $onDestroy() {
    console.log('CounterViewModel destroyed');
    // 清理资源、取消订阅等
  }

  /**
   * 生命周期：状态变化
   */
  $onStateChange(snapshot: CounterState) {
    console.log('State changed:', snapshot);
  }

  /**
   * 生命周期：错误处理
   */
  $onError(error: Error) {
    console.error('CounterViewModel error:', error);
  }

  /**
   * Action: 增加计数
   */
  increment() {
    this.state.count += this.state.step;
    this.state.history.push(this.state.count);
  }

  /**
   * Action: 减少计数
   */
  decrement() {
    this.state.count -= this.state.step;
    this.state.history.push(this.state.count);
  }

  /**
   * Action: 设置步长
   */
  setStep(step: number) {
    this.state.step = step;
  }

  /**
   * Action: 重置计数
   */
  reset() {
    this.$updateState({
      count: 0,
      history: [],
    });
  }

  /**
   * Action: 异步增加（模拟 API 调用）
   */
  async incrementAsync(delay = 1000) {
    await new Promise((resolve) => setTimeout(resolve, delay));
    this.increment();
  }

  /**
   * 计算属性：是否为正数
   */
  get isPositive(): boolean {
    return this.state.count > 0;
  }

  /**
   * 计算属性：是否为偶数
   */
  get isEven(): boolean {
    return this.state.count % 2 === 0;
  }

  /**
   * 计算属性：历史记录数量
   */
  get historyCount(): number {
    return this.state.history.length;
  }
}

/**
 * 创建单例实例
 */
export const counterViewModel = new CounterViewModel();
