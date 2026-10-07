# quantum-circuit-simulator-go

本地量子线路模拟与采样库：解析文本描述的量子线路，按顺序演化态矢量，支持中途测量与条件操作，结果可复现、可并发复用。

## 支持的电路范围

线路以文本 DSL 描述，每行一条，`#` 之后为注释：

```
qubits 2          # 量子比特数声明（必须且只能出现一次）
bits 2            # 经典比特数声明（必须且只能出现一次）
H 0               # 单比特门: H X Y Z S T
RX 0 1.5708       # 带参数旋转门: RX RY RZ（参数为弧度）
CNOT 0 1          # 双比特门: CNOT CZ SWAP
MEASURE 0 -> 0    # 测量量子比特 0，结果写入经典比特 0
IF 0==1 X 1       # 经典比特 0 等于 1 时才执行后续门
```

解析失败返回可区分的错误类型（均可用 `errors.As` 判定）：

| 错误类型 | 含义 |
|---|---|
| `circuit.ErrUnknownOp` | 未识别的门或操作 |
| `circuit.ErrOutOfRange` | 量子比特 / 经典比特下标越界 |
| `circuit.ErrDuplicateDecl` | `qubits` / `bits` 重复声明 |
| `circuit.ErrIncomplete` | 缺少声明、操作数不足、语法不完整等 |
| `circuit.ErrDuplicateOperand` | 同一门的多个操作数指向同一个量子比特（如 `CNOT 0 0`） |

任何非法结构都不会被静默忽略。特别地，一个门把多个操作数指向同一量子比特属于**非法线路**：
解析期直接报错；绕过 `Parse` 手工构造的 `Circuit` 也会在 `Run`/`Sample`/`NewSession`
入口经 `circuit.Validate` 复核并以同一错误类型拒绝，不会被当作正常线路算出看似合理的结果。

## 测量与随机语义

- 模拟采用态矢量（`[]complex128`，长度 2^n），量子比特 q 对应下标第 q 位（小端）。
- 测量按当前态的概率分布抽样（Born 规则），随后坍缩并归一化，影响后续演化；条件操作 `IF c==v` 依据当前经典比特取值决定是否生效。
- 随机性来自每次运行独立创建的 `rand.Rand`：
  - `Run(c, seed)` / `Sample(c, shots, seed)` 在同一 `(线路, seed)` 下结果**完全可复现**；
  - `Sample` 的多次采样共享同一条随机序列，因此整个计数表也可复现；
  - 不同种子下计数可不同，但分布仍符合线路的概率语义。
- 经典比特串按声明顺序输出，bit 0 在最左（如 `"01"` 表示 bit0=0, bit1=1）。

## 执行会话与检查点

除一次性 `Run`/`Sample` 外，还可以创建**执行会话**逐步推进线路，用于交互式调试与分支对比：

```go
sess, err := s.NewSession(c, 42)   // 与 Run 相同的校验与种子语义
for {
    r, err := sess.Step(0)          // 推进一条操作，并查询 q0 测 1 的概率
    // r.Step/r.Total 进度；r.Done 是否结束；
    // r.Bits 经典比特当前值（未确定的位为 -1）；
    // r.Prob1 查询比特测 1 的概率（Step(-1) 表示不查询，此时为 -1）
    if r.Done { break }
}
bits, err := sess.RunRemaining()    // 或随时一次性跑完剩余操作
```

- **可复现**：同一线路、同一种子、同样的推进过程，中间快照与最终结果完全可复现，且与 `Run(c, seed)` 一致。
- **检查点**：`cp := sess.Checkpoint()` 保存当前时刻的完整执行状态（态矢量、经典比特、随机源位置的深拷贝）；`s.Resume(cp)` 从中恢复出新会话继续跑，最终结果与一次性从头跑完**完全一致**。同一检查点可被多个会话**并发恢复**，各会话持有独立副本，互不污染。检查点仅在进程内有效，不落盘、不跨进程。
- **并发**：不同会话（含检查点恢复出的）之间不共享任何状态；单个 `Session` 不是并发安全的，请勿跨 goroutine 共享同一会话。
- 会话与检查点恢复受与 `Run` 相同的结构校验与资源上限约束，无法借此绕过限制。

## 资源限制与并发

- `sim.New(maxQubits)` 设定量子比特数上限；线路超出时 `Run`/`Sample`/`NewSession`/`Resume` 返回 `sim.ErrTooManyQubits`（可用 `errors.As` 判定），不会尝试分配超额内存。
- 上限归一化规则（处理方式一致且可预期，只影响上限取值，不会因此拒绝本可运行的线路）：
  - `maxQubits <= 0`（未显式给出上限）→ 使用缺省保护上限 `sim.DefaultMaxQubits`（20）；
  - `maxQubits > sim.HardMaxQubits` → 收敛到全局硬上限 `sim.HardMaxQubits`（26，约 1GiB 态矢量）；
  - 任何调用都无法突破 `HardMaxQubits`。
- `Simulator` 不保存任何运行态，所有状态（态矢量、经典比特、随机源）都在单次调用/单个会话内独立分配，因此同一实例可被多个 goroutine **并发复用**，运行之间互不污染、无数据竞争（测试以 `-race` 验证）。
- 随机源使用库内实现的可快照 SplitMix64（`sim` 内部），保证一次性运行、逐步会话、检查点续跑在同一种子下共享同一条随机序列。

## 本地验证

```bash
go build ./...
go test ./...          # 单元测试（日志含输入电路与判定依据）
go test ./... -race    # 并发数据竞争检测
go test ./sim/ -v -run TestConcurrentReuse
go test ./sim/ -v -run 'TestSession|TestCheckpoint'   # 会话逐步推进与检查点续跑
go test ./sim/ -v -run 'TestLimit|TestResume'         # 资源上限归一化与恢复限制
go test ./circuit/ -v -run TestParseRejections        # 非法线路（含重复操作数）拒绝
```

## 示例

```go
c, err := circuit.Parse(`
qubits 2
bits 2
H 0
CNOT 0 1
MEASURE 0 -> 0
MEASURE 1 -> 1
`)
if err != nil { /* errors.As 区分失败原因 */ }

s := sim.New(20)
res, err := s.Sample(c, 1000, 42)
// res.Counts ≈ map[00:~500 11:~500]，同种子重跑结果一致
```
