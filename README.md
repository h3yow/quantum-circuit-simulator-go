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

任何非法结构都不会被静默忽略。

## 测量与随机语义

- 模拟采用态矢量（`[]complex128`，长度 2^n），量子比特 q 对应下标第 q 位（小端）。
- 测量按当前态的概率分布抽样（Born 规则），随后坍缩并归一化，影响后续演化；条件操作 `IF c==v` 依据当前经典比特取值决定是否生效。
- 随机性来自每次运行独立创建的 `rand.Rand`：
  - `Run(c, seed)` / `Sample(c, shots, seed)` 在同一 `(线路, seed)` 下结果**完全可复现**；
  - `Sample` 的多次采样共享同一条随机序列，因此整个计数表也可复现；
  - 不同种子下计数可不同，但分布仍符合线路的概率语义。
- 经典比特串按声明顺序输出，bit 0 在最左（如 `"01"` 表示 bit0=0, bit1=1）。

## 资源限制与并发

- `sim.New(maxQubits)` 设定量子比特数上限；线路超出时 `Run`/`Sample` 返回 `sim.ErrTooManyQubits`（可用 `errors.As` 判定），不会尝试分配超额内存。
- `Simulator` 不保存任何运行态，所有状态（态矢量、经典比特、随机源）都在单次调用的栈/堆上独立分配，因此同一实例可被多个 goroutine **并发复用**，运行之间互不污染、无数据竞争（测试以 `-race` 验证）。

## 本地验证

```bash
go build ./...
go test ./...          # 单元测试（日志含输入电路与判定依据）
go test ./... -race    # 并发数据竞争检测
go test ./sim/ -v -run TestConcurrentReuse
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
