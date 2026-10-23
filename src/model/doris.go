package model

// DBTypeDoris 数据源类型：0 mysql（默认）、1 pg、2 mongodb、3 redis、4 doris。
//
// Doris 兼容 MySQL 协议（FE 的 MySQL 协议端口，mysql driver 可直连），
// 但建表 DDL 是自有语法（DISTRIBUTED BY / PROPERTIES / DUPLICATE|UNIQUE|AGGREGATE KEY），
// MySQL 解析器解析不了。因此 Doris 的审核走混合路径：
// DML/查询用 MySQL 解析器 + advisor，建表 DDL 用引擎侧自研的 internal/dorischeck。
const DBTypeDoris = 4
