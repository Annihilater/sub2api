/**
 * useEscapeStack — 全局 ESC 按键优先级栈
 *
 * 规则：
 *  - 调用 pushEscape(handler) 将回调压栈；返回 pop 函数用于注销。
 *  - 每次 ESC 按下只触发**栈顶**（最近注册）的一个回调。
 *  - 触发后自动阻止事件继续传播，保证不会同时触发多层。
 *  - 组件卸载时务必调用返回的 pop 函数，或在 onUnmounted 里调用。
 *
 * 使用示例：
 *   const pop = pushEscape(() => { myVisible.value = false })
 *   onUnmounted(pop)
 *
 *   // 或者在 setup 里用 watchEffect 自动管理：
 *   watchEffect((onCleanup) => {
 *     if (!visible.value) return
 *     const pop = pushEscape(() => { visible.value = false })
 *     onCleanup(pop)
 *   })
 */

type EscapeHandler = () => void

const stack: EscapeHandler[] = []

let listenerAttached = false

function onKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape' || stack.length === 0) return
  event.preventDefault()
  event.stopPropagation()
  // 调用栈顶回调（但不自动 pop，由业务方决定是否关闭后 pop）
  stack[stack.length - 1]()
}

function ensureListener() {
  if (listenerAttached) return
  document.addEventListener('keydown', onKeydown, true) // capture 优先于 BaseDialog 的 bubble 监听
  listenerAttached = true
}

export function pushEscape(handler: EscapeHandler): () => void {
  ensureListener()
  stack.push(handler)
  return function pop() {
    const idx = stack.lastIndexOf(handler)
    if (idx !== -1) stack.splice(idx, 1)
  }
}
