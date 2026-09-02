# Day3 提示词

## Claude

> [https://github.com/KDZZZZZZ/short-term/tree/feature/backend-microservices-mvp](https://github.com/KDZZZZZZ/short-term/tree/feature/backend-microservices-mvp)拉取然后基于文档继续开发

## Codex

> [https://github.com/KDZZZZZZ/short-term](https://github.com/KDZZZZZZ/short-term)拉取这个repo，并且配置后端开发环境

> 你先再看一遍文档和现在实现到哪里了，然后继续实现文档

> 数据存储应该都是在数据库吧不是在内存里，密码有没有明文存储

这些提示词对应 Platform、Account、Marketplace、持久化和认证基础能力。

## DOCX 提问记录

- **Q037 | 2026-08-12–08-14 | 摘要：**「为何 result 这样写（error）」 *（Rust 代码片段讨论，当前上下文没有完整原句。）*
- **Q038 | 2026-08-12–08-14 | 摘要：**「trait 何时用」
- **Q039 | 2026-08-12–08-14 | 摘要：**「如何包装未知签名函数」
- **Q040 | 2026-08-12–08-14 | 摘要：**「宏/元编程在类型方面用途」
- **Q041 | 2026-08-12–08-14 | 原话（摘要转录）：**「你这里都没用 macro」
- **Q042 | 2026-08-17 | 摘要：**「前台命令结束为何仍需手动杀进程」
- **Q043 | 2026-08-17 | 摘要：**「沙箱的正确行为是什么」
- **Q044 | 2026-08-17 | 约束（非问题）：**「task 完成后杀死所有进程，其余与 pi 一致」
- **Q045 | 2026-08-17 | 摘要：**「POSIX FS 是否必要、overlay/Absorb 如何处理删除和稀疏文件」
- **Q046 | 2026-08-17 | 摘要：**「React 如何异步等待工具与模型」
- **Q047 | 2026-08-21 | 原话：**「codex的sdk是不是今天开源了，在哪用」
- **Q048 | 2026-08-24 | 原话：**「如何增加命令执行并发效率，有哪些思路」
- **Q049 | 2026-08-22 | 原话：**「沙箱的定义是什么，https://github.com/KDZZZZZZ/threadmill/tree/dev-native这个已经算有沙箱了吧」
- **Q050 | 2026-08-22 | 原话：**「这个沙箱和主流agent沙箱比有什么特殊」
- **Q051 | 2026-08-20 | 摘要：**「生产 Go 项目是否保留测试代码」

## DOCX 整理版提示词

> OpenAPI 已经固定，请实现只包含跨服务共享能力的 Platform：配置加载、ID、日志、Tracing、错误模型、认证 token、PostgreSQL pool/migration、health、gRPC server/client 公共封装。运行时必须有明确的进程回收、命令拦截、虚拟文件视图和 fail-closed 隔离行为；每个能力都要有最小测试、资源上限和可观测日志，不要把业务规则塞进 Platform。
