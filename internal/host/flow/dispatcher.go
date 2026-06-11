package flow

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/voocel/agentcore"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

// Dispatcher 订阅 Coordinator 事件，在子代理返回时计算路由并下达 Host 指令。
//
// 生命周期：Attach 返回一个 detach 函数；关闭 Host 时调用释放订阅。
type Dispatcher struct {
	coordinator *agentcore.Agent
	store       *storepkg.Store

	enabled atomic.Bool // 由 Host 控制是否派发（启动完成前应关）

	// 重复追踪：记住最近一次派发的 Agent+Task 与连续下达次数。
	// 同一指令重复计算（子代理返回后状态未推进，Route 重算结果不变）不静默吞掉，
	// 而是带次数事实重发——"路由结果连续 N 次相同"是只有 Host 能观测到的事实；
	// 若沉默，Coordinator 会陷入"禁止自行决定下一步"（coordinator.md）与
	// "禁止停机"（StopGuard）的双重矛盾，自由发挥即 #24 类 freelance 死循环。
	// 裁定权仍在 LLM：重发消息只附事实与核对许可，不设阈值、不熔断（架构 §10.13）。
	// 消息因带次数而互不相同，不会把字面相同的指令重复压进 followUpQ。
	lastMu   sync.Mutex
	lastSent *Instruction
	repeats  int
}

// NewDispatcher 创建 Dispatcher。使用前需调用 Attach 订阅事件。
func NewDispatcher(coordinator *agentcore.Agent, store *storepkg.Store) *Dispatcher {
	d := &Dispatcher{coordinator: coordinator, store: store}
	return d
}

// Enable 打开路由派发；关闭时 EventToolExecEnd 到达不会发 FollowUp。
// Host 在 Start/Resume 完成首条 prompt 之后启用，避免与启动流程冲突。
func (d *Dispatcher) Enable() { d.enabled.Store(true) }

// Attach 订阅 Coordinator 事件；返回的函数在关闭时调用以解绑。
func (d *Dispatcher) Attach() func() {
	return d.coordinator.Subscribe(d.handle)
}

func (d *Dispatcher) handle(ev agentcore.Event) {
	if !d.enabled.Load() {
		return
	}
	// 精确触发点：子代理调用成功返回。
	// 不用 EventTurnEnd，因为 agentcore 每次 LLM call 完成都会 emit TurnEnd，
	// 会把同一条指令重复压进 followUpQ；查询类 Steer 由 coordinator.md 约束在
	// 同一 turn 内继续调 subagent，从而命中这个触发点。
	if ev.Type != agentcore.EventToolExecEnd || ev.Tool != "subagent" || ev.IsError {
		return
	}
	d.Dispatch()
}

// Dispatch 立即计算路由并下达指令；可被 Host 在特殊时机（如 Resume 后）主动调用。
func (d *Dispatcher) Dispatch() {
	state := LoadState(d.store)
	inst := Route(state)
	if inst == nil {
		return
	}
	n := d.trackRepeat(inst)
	// Writer 任务：在派发同一刻把章节标为进行中，UI 右侧大纲立即反映"▸ 进行中"，
	// 不用等 plan_chapter 真正执行（plan_chapter 会再调一次 StartChapter，幂等）。
	if inst.Agent == "writer" && inst.Chapter > 0 && d.store != nil {
		if err := d.store.Progress.StartChapter(inst.Chapter); err != nil {
			slog.Warn("flow router pre-mark in-progress failed", "module", "host.flow", "chapter", inst.Chapter, "err", err)
		}
	}
	msg := formatDispatchMessage(inst, n)
	slog.Debug("flow router dispatch", "module", "host.flow", "agent", inst.Agent, "reason", inst.Reason, "repeat", n)
	d.coordinator.FollowUp(agentcore.UserMsg(msg))
}

// formatDispatchMessage 组装下达给 Coordinator 的指令消息。
// n>1 时附加重复事实——告知"上次派发后路由事实未变化"并放开核对许可，
// 让 LLM 自己裁定照常执行还是改派；不在 Host 层做任何强制分支。
func formatDispatchMessage(inst *Instruction, n int) string {
	msg := FormatMessage(inst)
	if n > 1 {
		msg += fmt.Sprintf("\n（注意：本指令为第 %d 次下达——上次派发后路由事实未变化。本次允许先调 novel_context 核对事实，再裁定照常执行或改派其它子代理。）", n)
	}
	return msg
}

// trackRepeat 记录连续相同指令的下达次数并返回当前次数（1 = 新指令）。
// 用 Agent+Task 相等性（不比 Reason，因为 Reason 是给人看的辅助文本）。
func (d *Dispatcher) trackRepeat(next *Instruction) int {
	d.lastMu.Lock()
	defer d.lastMu.Unlock()
	if d.lastSent != nil && d.lastSent.Agent == next.Agent && d.lastSent.Task == next.Task {
		d.repeats++
		return d.repeats
	}
	cp := *next
	d.lastSent = &cp
	d.repeats = 1
	return 1
}

// ResetRepeat 清空重复追踪。Resume / 新 Start 时 Host 调用，
// 确保恢复或新建后首条指令以"第 1 次"语义下达。
func (d *Dispatcher) ResetRepeat() {
	d.lastMu.Lock()
	defer d.lastMu.Unlock()
	d.lastSent = nil
	d.repeats = 0
}
