# Bug Reproduction

## 包的性质

当前 test_model_fix 保存的是被测模型修复后的结果源码，不是初始含 Bug 源码。要复现原始缺陷，必须检出下面固定的 parent SHA；不要在当前修复结果源码上期待重新出现修复前失败。生成系统使用的可信验证补丁和完整验证日志仅在本地留存，不提交到结果分支。

## 问题现象

两台 worker 同时扫描超期隐患，会给同一家企业发送两条相同催办，并各写一条审计。请修复任务领取，保证一条超期记录同一时刻只由一个 worker 处理，处理失败后仍可重试。现有并发测试文件禁止增删修改，race 检测和重复通知断言也必须保留。

## 含 Bug 版本

- 仓库：11DingKing/chargeguard-bug-29
- 仓库地址：https://github.com/11DingKing/chargeguard-bug-29.git
- parent SHA：154989d6eaeb47111ef6a2709a0ef4d6ce5c2604

## 复现步骤

```bash
git clone -- https://github.com/11DingKing/chargeguard-bug-29.git bug-repro
cd bug-repro
git checkout --detach 154989d6eaeb47111ef6a2709a0ef4d6ce5c2604
go test ./internal/httpapi -run TestTaskBehavior -count=1
```

## 双架构完整错误信息

### linux/amd64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test ./internal/httpapi -run TestTaskBehavior -count=1
--- FAIL: TestTaskBehavior (0.01s)
    task_behavior_test.go:39: ok=2 conflict=0
FAIL
FAIL	chargeguard/internal/httpapi	0.072s
FAIL

```

stderr：

```text
(empty)
```

### linux/arm64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test ./internal/httpapi -run TestTaskBehavior -count=1
--- FAIL: TestTaskBehavior (0.00s)
    task_behavior_test.go:39: ok=2 conflict=0
FAIL
FAIL	chargeguard/internal/httpapi	0.003s
FAIL

```

stderr：

```text
(empty)
```

## 通过条件

修复后，在题面描述的触发条件下应得到预期业务结果且不再出现原始症状；定向验证命令修复前必须失败、应用修复后必须通过，相关回归和仓库全量测试必须通过；不得新增、删除或修改测试文件，不得跳过测试、降低断言或绕过目标逻辑。
