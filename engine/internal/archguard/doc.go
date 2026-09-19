// Package archguard 存放"架构护栏"测试：用来自动化拦住内化 Bytebase 代码
// 在同步/升级时产生的两类破坏。
//
//  1. 耦合边界：除适配层外，任何包都不得 import engine/internal/bytebase。
//     这保证上游 API 漂移的爆炸半径被限制在少数几个文件内。
//  2. 规则映射一致性：server/rules.go 引用的规则类型必须同时存在于
//     SQLReviewRule_Type 枚举与 Bytebase MySQL 的注册表中，避免运行期
//     才以 "advisor: unknown advisor" 的形式暴露。
//
// 本包不 import 任何内化代码，只读源码做静态检查，因此不违反第 1 条。
package archguard
