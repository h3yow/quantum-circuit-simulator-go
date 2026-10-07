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
| `circuit.ErrDuplicateQubit` | 同一门的多个操作数指向同一量子比特（如 `CNOT 0 0`），属非法线路 |

任何非法结构都不会被静默忽略；特别地，多比特门的操作数重复（`CNOT 0 0`、`SWAP 1 1`、条件门内同样生效）会在解析期以 `ErrDuplicateQubit` 拒绝，而不会算出看似合理的结果。

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
sess, err := s.NewSession(c, 42)   // 同 (线路, seed) 完全可复现
for {
    r, err := sess.Step()          // 推进一条操作
    if err != nil { /* ... */ }
    // r.Index/r.Total 进度；r.Bits 当前经典比特；r.Determined 各比特是否已被测量写入
    p1, err := sess.QubitProbability(0) // 查询指定量子比特当前测得 1 的概率
    if r.Done { break }
}
bits, _ := sess.RunToEnd()         // 也可以从当前位置直接跑完
```

- 会话独占自己的态矢量、经典比特与随机序列：同一 `(线路, seed)` 同样推进过程下中间与最终结果完全可复现；多个会话并发推进互不污染。
- `sess.Checkpoint()` 把当前时刻的执行状态（态矢量、经典比特、随机序列位置、进度）存为**检查点**（深拷贝，仅进程内有效，不落盘）；`s.Restore(cp)` 恢复出新会话继续跑，最终经典比特结果与一次性从头跑完**完全一致**。同一检查点可被多个会话并发恢复，彼此独立。
- 会话与检查点同样受量子比特上限约束：`NewSession`/`Restore` 在线路超限时返回 `sim.ErrTooManyQubits`，不能借会话绕过资源保护。

## 资源限制与并发

- `sim.New(maxQubits)` 设定量子比特数上限；线路超出时 `Run`/`Sample`/`NewSession`/`Restore` 返回 `sim.ErrTooManyQubits`（可用 `errors.As` 判定），不会尝试分配超额内存。
- 上限按统一规则钳制，生效值保存在 `Simulator.MaxQubits` 可直接读取：
  - `maxQubits <= 0`（未显式指定）：使用缺省保护 `sim.DefaultMaxQubits`（20）；
  - `maxQubits > sim.HardMaxQubits`（30）：钳制到该全局硬上限，任何调用都无法突破；
  - 不合理的上限只影响钳制后的生效值，不会因此拒绝正常线路。
- `Simulator` 不保存任何运行态，所有状态（态矢量、经典比特、随机源）都在单次调用或单个会话内独立分配，因此同一实例可被多个 goroutine **并发复用**，运行之间互不污染、无数据竞争（测试以 `-race` 验证）。

## 本地验证

```bash
go build ./...
go test ./...          # 单元测试（日志含输入电路与判定依据）
go test ./... -race    # 并发数据竞争检测
go test ./sim/ -v -run TestConcurrentReuse
go test ./sim/ -v -run 'TestSession|TestCheckpoint'   # 会话与检查点用例
go test ./sim/ -v -run 'TestLimit|TestHardCap'        # 资源上限规则用例
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
