# teleagent2api 会话总结

## 时间
2026-06-24 ~ 2026-06-25

## 已配置
- 监听 `:10000`，API Key `sk-teleagent-key`
- 三个模型：chat-lite (100K/16384)、chat-pro (192K/65536)、chat-flash (192K/65536)
- 接入 NewAPI（`D:\project1\newapi`）
- PowerShell 快速启动：`tele` / `teleagents` → teleagent2api；`newa` / `newapi` → NewAPI

## 已修 Bug（24 个）

### 严重级（7/8）
- statusWriter 加 Flush() — SSE 流式可推数据
- credentials 文件读取修复
- 空凭证 %0 守卫
- 超时 300s→1800s
- model 映射回请求值
- 4xx 错误转 OpenAI `{"error":{...}}` 格式
- finish_reason 空字符串防护

### 高级（3/3）
- Flush 在 [DONE] 前调用
- signal.Notify 提前注册
- 非 SSE 响应降级 JSON

### 中级（9/23）
- json.Marshal(choices) 错误检查
- ReadTimeout 最小值 1s 保护
- 环境变量负超时守卫
- max_tokens 零负值处理
- Auth Bearer 大小写不敏感
- 非 data: SSE 行过滤
- WWW-Authenticate 加 realm
- fc.RetryCount 允许 0
- Connection close 限 HTTP/1.x

### 低级（5/37）
- mustHexToken 不 panic
- RetryCount 上限 10
- 未知模型 max_tokens 上限 65536
- 可预测回退 RequestID
- [DONE] 空白字符容忍

## 未修重点
- 流式 tool_calls 丢弃（高风险，需重构 adapter）
- 剩余 47 个中低级别 bug（详见 TELEAGENT2API_RESEARCH.md）

## 文档
- `TELEAGENT2API_RESEARCH.md` v2.2（900+行，含完整分析）
- 备份：`C:\Temp\opencode\teleagent2api_backup_20260624_235307`

---

## 2026-06-25 大修（31 个 bug，全部修完）

### 本次修了什么
- **C2/#6 流式 tool_calls** — 提取 delta.tool_calls + buildToolCall 透传
- **7 个中级 bug** — #17 #18 #22 #23 #26 #32 #65
- **22 个低级 bug** — #15 #20 #21 #27 #35 #36 #40 #46 #48 #50 #53 #54 #58 #60 #61 #62 #63 #64 #69 #71 + 编译错误 + usage 重复发射 + 删死代码

### 当前状态
| 级别 | 总数 | 已修 | 剩余 |
|------|------|------|------|
| 严重 | 8 | 8 | **0** |
| 高级 | 3 | 3 | **0** |
| 中级 | 23 | 23 | **0** |
| 低级 | 37 | 21 | 16 |
| **总计** | **71** | **55** | **16** |

### 文档
- `TELEAGENT2API_RESEARCH.md` v2.3（已更新统计+修复记录）
- 代码已推送至 `1521036689/teleagent2api`
