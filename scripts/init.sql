-- =====================================================================
-- Yearning 数据库初始化脚本
-- 导出源：117.72.197.23:33066 / Yearning_go（已初始化完成的库，共 17 张表）
-- 用途：更换数据库时执行本脚本即可完成「建表 + 初始数据」，无需再跑 ./Yearning install
--
-- 用法：
--   mysql -h <host> -P <port> -u <user> -p < init.sql
--   或在 mysql 客户端内： source /path/to/init.sql
--
-- 说明：
--   1. 脚本自带 CREATE DATABASE IF NOT EXISTS + USE，库名见下两行；
--      若目标库名不同，请同时修改这两处的库名，或干脆删掉这两行手动先建库。
--   2. 服务端需支持 utf8mb4（脚本已 SET NAMES utf8mb4）。
--   3. 初始账号 admin，初始密码 Yearning_admin。
--      password 字段是 Django 格式哈希（算法+盐+摘要），请勿手工修改。
--   4. 执行后直接启动 Yearning 即可；启动时的 UpdateData() 会自动校验并补齐结构。
-- =====================================================================

CREATE DATABASE IF NOT EXISTS `Yearning_go` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;
USE `Yearning_go`;
SET NAMES utf8mb4;

-- ===================== 1. 表结构 =====================

DROP TABLE IF EXISTS `core_accounts`;
CREATE TABLE `core_accounts` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `username` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `password` varchar(150) COLLATE utf8mb4_general_ci NOT NULL,
  `department` varchar(50) COLLATE utf8mb4_general_ci DEFAULT NULL,
  `real_name` varchar(50) COLLATE utf8mb4_general_ci DEFAULT NULL,
  `email` varchar(50) COLLATE utf8mb4_general_ci DEFAULT NULL,
  `is_recorder` tinyint NOT NULL DEFAULT '2',
  PRIMARY KEY (`id`),
  KEY `user_idx` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

DROP TABLE IF EXISTS `core_auto_tasks`;
CREATE TABLE `core_auto_tasks` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `source` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `source_id` varchar(200) COLLATE utf8mb4_general_ci NOT NULL,
  `data_base` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `table` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `tp` tinyint NOT NULL,
  `affectrow` int DEFAULT NULL,
  `status` tinyint DEFAULT NULL,
  `task_id` varchar(200) COLLATE utf8mb4_general_ci NOT NULL,
  `id_c` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  PRIMARY KEY (`id`),
  KEY `source_idx` (`source_id`),
  KEY `task_idx` (`task_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

DROP TABLE IF EXISTS `core_data_sources`;
CREATE TABLE `core_data_sources` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `id_c` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `source` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `ip` varchar(200) COLLATE utf8mb4_general_ci NOT NULL,
  `port` int NOT NULL,
  `username` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `password` varchar(150) COLLATE utf8mb4_general_ci NOT NULL,
  `is_query` tinyint NOT NULL,
  `flow_id` int NOT NULL,
  `source_id` varchar(200) COLLATE utf8mb4_general_ci NOT NULL,
  `exclude_db_list` varchar(200) COLLATE utf8mb4_general_ci NOT NULL,
  `insulate_word_list` varchar(200) COLLATE utf8mb4_general_ci NOT NULL,
  `principal` varchar(150) COLLATE utf8mb4_general_ci NOT NULL,
  `ca_file` longtext COLLATE utf8mb4_general_ci,
  `cert` longtext COLLATE utf8mb4_general_ci,
  `key_file` longtext COLLATE utf8mb4_general_ci,
  `db_type` int NOT NULL,
  `rule_id` int NOT NULL,
  PRIMARY KEY (`id`),
  KEY `source_idx` (`source_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

DROP TABLE IF EXISTS `core_global_configurations`;
CREATE TABLE `core_global_configurations` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `authorization` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `message` json DEFAULT NULL,
  `other` json DEFAULT NULL,
  `stmt` tinyint NOT NULL DEFAULT '0',
  `audit_role` json DEFAULT NULL,
  `board` longtext COLLATE utf8mb4_general_ci,
  `ai` json DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

DROP TABLE IF EXISTS `core_graineds`;
CREATE TABLE `core_graineds` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `username` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `group` json DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `user_idx` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

DROP TABLE IF EXISTS `core_order_comments`;
CREATE TABLE `core_order_comments` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `work_id` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `username` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `content` longtext COLLATE utf8mb4_general_ci,
  `time` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  PRIMARY KEY (`id`),
  KEY `workId_idx` (`work_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

DROP TABLE IF EXISTS `core_query_orders`;
CREATE TABLE `core_query_orders` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `work_id` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `username` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `date` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `approval_time` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `text` longtext COLLATE utf8mb4_general_ci NOT NULL,
  `assigned` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `real_name` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `export` tinyint NOT NULL,
  `source_id` varchar(200) COLLATE utf8mb4_general_ci NOT NULL,
  `status` tinyint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `workId_idx` (`work_id`),
  KEY `source_idx` (`source_id`),
  KEY `status_idx` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

DROP TABLE IF EXISTS `core_query_records`;
CREATE TABLE `core_query_records` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `work_id` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `sql` longtext COLLATE utf8mb4_general_ci NOT NULL,
  `ex_time` int NOT NULL,
  `time` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `source` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `schema` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  PRIMARY KEY (`id`),
  KEY `workId_idx` (`work_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

DROP TABLE IF EXISTS `core_role_groups`;
CREATE TABLE `core_role_groups` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `permissions` json DEFAULT NULL,
  `group_id` varchar(200) COLLATE utf8mb4_general_ci NOT NULL,
  PRIMARY KEY (`id`),
  KEY `group_idx` (`group_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

DROP TABLE IF EXISTS `core_rollbacks`;
CREATE TABLE `core_rollbacks` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `work_id` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `sql` longtext COLLATE utf8mb4_general_ci NOT NULL,
  PRIMARY KEY (`id`),
  KEY `workId_idx` (`work_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

DROP TABLE IF EXISTS `core_rule_set_histories`;
CREATE TABLE `core_rule_set_histories` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `rule_id` int NOT NULL,
  `before` json DEFAULT NULL,
  `after` json DEFAULT NULL,
  `operator` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `note` varchar(255) COLLATE utf8mb4_general_ci DEFAULT NULL,
  `created_at` varchar(30) COLLATE utf8mb4_general_ci NOT NULL,
  PRIMARY KEY (`id`),
  KEY `rule_idx` (`rule_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

DROP TABLE IF EXISTS `core_rules`;
CREATE TABLE `core_rules` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `desc` longtext COLLATE utf8mb4_general_ci NOT NULL,
  `audit_role` json DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

DROP TABLE IF EXISTS `core_sql_orders`;
CREATE TABLE `core_sql_orders` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `work_id` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `username` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `status` tinyint NOT NULL,
  `type` tinyint NOT NULL,
  `backup` tinyint NOT NULL,
  `id_c` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `source` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `source_id` varchar(200) COLLATE utf8mb4_general_ci NOT NULL,
  `data_base` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `table` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `date` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `sql` longtext COLLATE utf8mb4_general_ci NOT NULL,
  `text` longtext COLLATE utf8mb4_general_ci NOT NULL,
  `assigned` varchar(550) COLLATE utf8mb4_general_ci NOT NULL,
  `delay` varchar(50) COLLATE utf8mb4_general_ci NOT NULL DEFAULT 'none',
  `real_name` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `execute_time` varchar(50) COLLATE utf8mb4_general_ci DEFAULT NULL,
  `current_step` int DEFAULT NULL,
  `relevant` json DEFAULT NULL,
  `osc_info` longtext COLLATE utf8mb4_general_ci,
  `file` varchar(200) COLLATE utf8mb4_general_ci NOT NULL,
  `batch_id` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  PRIMARY KEY (`id`),
  KEY `batch_idx` (`batch_id`),
  KEY `workId_idx` (`work_id`),
  KEY `query_idx` (`username`),
  KEY `source_idx` (`source_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

DROP TABLE IF EXISTS `core_sql_records`;
CREATE TABLE `core_sql_records` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `work_id` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `sql` longtext COLLATE utf8mb4_general_ci NOT NULL,
  `state` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `affectrow` int NOT NULL,
  `time` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `error` longtext COLLATE utf8mb4_general_ci,
  PRIMARY KEY (`id`),
  KEY `workId_idx` (`work_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

DROP TABLE IF EXISTS `core_total_tickets`;
CREATE TABLE `core_total_tickets` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `date` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `total_order` int NOT NULL,
  `total_query` int NOT NULL,
  PRIMARY KEY (`id`),
  KEY `date_idx` (`date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

DROP TABLE IF EXISTS `core_workflow_details`;
CREATE TABLE `core_workflow_details` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `work_id` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `username` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `time` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `action` varchar(550) COLLATE utf8mb4_general_ci NOT NULL,
  PRIMARY KEY (`id`),
  KEY `workId_idx` (`work_id`),
  KEY `query_idx` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

DROP TABLE IF EXISTS `core_workflow_tpls`;
CREATE TABLE `core_workflow_tpls` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `source` varchar(50) COLLATE utf8mb4_general_ci NOT NULL,
  `steps` json DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `source_idx` (`source`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- ===================== 2. 初始数据 =====================

-- core_accounts：1 行
INSERT INTO `core_accounts` (`id`, `username`, `password`, `department`, `real_name`, `email`, `is_recorder`) VALUES
  (1, 'admin', 'pbkdf2_sha256$600000$7NBTVWL8BiSQqxJG$KUCAkFj5WNgHp+DlUiFsu/lZhJg8DVXaVH90cVVQD8c=', 'DBA', '超级管理员', '', 2);

-- core_global_configurations：1 行
INSERT INTO `core_global_configurations` (`id`, `authorization`, `message`, `other`, `stmt`, `audit_role`, `board`, `ai`) VALUES
  (1, 'global', '{"key": "", "ssl": false, "ding": false, "host": "", "mail": false, "port": 25, "user": "", "to_user": "", "password": "", "web_hook": "", "push_type": false}', '{"idc": ["Aliyun", "AWS"], "limit": 1000, "query": false, "domain": "", "export": false, "register": false, "ex_query_time": 60, "manual_execute": false}', 0, '{"IsOSC": false, "OSCExpr": "", "OscSize": 0, "DMLOrder": false, "DMLWhere": false, "DDLMaxKey": 5, "DMLSelect": false, "RuleLevel": null, "PRIRollBack": false, "MaxAffectRows": 1000, "DDLMaxKeyParts": 5, "DDLTablePrefix": "", "DMLTransaction": false, "SupportCharset": "", "AllowCreateView": false, "CheckIdentifier": false, "MaxTableNameLen": 10, "MustHaveColumns": "", "AllowSpecialType": false, "DDLIndexNameSpec": false, "DDLMaxCharLength": 10, "DDLMultiToCommit": false, "DMLInsertColumns": false, "DMLMaxInsertRows": 10, "MaxDDLAffectRows": 0, "SupportCollation": "", "DDLAllowPRINotInt": false, "DDLForbidTruncate": false, "DDLPrimaryKeyMust": false, "DMLAllowLimitSTMT": false, "DDLAllowColumnType": false, "DDLAllowMultiAlter": false, "DDLEnableDropTable": false, "DMLAllowInsertNull": false, "DDLCheckFloatDouble": false, "DDLEnableForeignKey": false, "DDLEnablePrimaryKey": false, "AllowCreatePartition": false, "DDLCheckTableComment": false, "DDLCheckColumnDefault": false, "DDLEnableDropDatabase": false, "DDlCheckColumnComment": false, "DDLCheckColumnNullable": false, "DDLEnableAutoIncrement": false, "DDLEnableNullIndexName": false, "DDLColumnsMustHaveIndex": "", "DDLEnableAcrossDBRename": false, "DMLInsertMustExplicitly": false, "DMLWhereExprValueIsNull": false, "DDLImplicitTypeConversion": false, "DDLEnableAutoincrementInit": false, "AllowCrateViewWithSelectStar": false, "DDLAllowChangeColumnPosition": false, "DDLEnableAutoincrementUnsigned": false}', '', '{"model": "gpt-3.5-turbo", "top_p": 0, "api_key": "", "base_url": "https://api.openai.com/v1", "proxy_url": "", "max_tokens": 2500, "temperature": 0, "advisor_prompt": "\\nSQL Optimization expert assistant \\n \\nWelcome to SQL Optimization Expert Assistant! Whether you\'re a database administrator, data analyst, or software developer, we\'ll walk you through a series of questions and recommendations to help you optimize your SQL queries for greater efficiency and performance. Please answer the following questions or implement relevant recommendations based on your specific situation. \\n \\nQuery analysis \\n \\nDescribe the SQL query you want to optimize. Provide key parts of the query, such as SELECT, FROM, and WHERE clauses. \\nHave you used EXPLAIN (or an equivalent tool) to analyze the query execution plan? If so, briefly describe the key points of the execution plan, such as scan type, join order, and so on. \\n \\nIndex optimization \\n \\nPlease list the existing indexes of all the tables involved in your query. Have you considered adding indexes to key columns in your query to improve performance? \\nIf you already have indexes, is the query making good use of them? Consider using forced indexing (if the database supports it) to test different indexing strategies. \\n \\nQuery structure \\n \\nDoes your query use subqueries, Ctes (common expressions), temporary tables, or views? Consider whether it is possible to improve performance by refactoring queries, such as converting subqueries into joins. \\nExamine whether it is possible to optimize queries by merging them, eliminating redundant JOIN operations, or using a more efficient aggregation strategy. \\n \\nAdvanced optimization strategy \\n \\nHave you considered partitioning tables to optimize queries on large data sets? \\nFor very complex or data-heavy queries, have you considered using materialized views or precomputed summary tables to improve performance? \\nReview the SQL statements provided below based on the above questions and suggestions, and try the optimization strategy accordingly. If you encounter specific problems or need further guidance during the optimization process, please provide details and we will provide more specific recommendations based on your situation. \\n \\nRelated table structure: \\n \\n{{tables_info}} \\n \\nSQL: \\n \\n{{sql}} \\n \\nReply Language: {{lang}}\\n\\n", "sql_gen_prompt": "\\nSQL statement generation assistant \\n \\nNow you will play the role of a professional DBA and generate the corresponding MYSQL SQL statement from the user\'s description. \\n \\nTable structure: {{tables_info}}\\n\\nSQL: {{sql}}\\n\\nUse the markdown format\\n\\nReply Language: {{lang}}\\n\\n", "presence_penalty": 0, "sql_agent_prompt": "\\n\\n # Role: MySQL language teaching specialist \\n \\n## Profile: \\n \\n- Language: {{lang}} \\n- Description: As an expert in MySQL language teaching with rich experience in MySQL teaching, you are able to teach MySQL knowledge in a fascinating way, patiently answer students\' various questions in a detailed and comprehensive way, remind students of mistakes or confusion in the process of learning MySQL statements, and explain your knowledge through example codes and detailed comments. Help students review MySQL exam, learn MySQL language, develop good MySQL language programming habits, and cultivate excellent MySQL language programming ability. \\n \\n### Skill: \\n \\n1. Rich experience in MySQL teaching \\n2. Engaging teaching methods \\n3. Patient, detailed and comprehensive answer ability \\n4. Remind students of mistakes or confusion \\n5. Illustrate knowledge through example codes and detailed comments \\n \\n## Goals: \\n \\n1. Guide students to master the basic knowledge of MySQL \\n2. Help students understand complex MySQL concepts \\n3. Provide detailed code examples and comments \\n4. Remind students of common mistakes and confusion \\n5. Help students study for the MySQL exam \\n \\n## Constrains: \\n \\n1. Teaching in {{lang}} \\n2. Provide detailed code examples and comments \\n3. Answer students\' questions patiently \\n4. Remind students of common mistakes and confusion \\n5. Help students develop good MySQL programming habits \\n \\n## OutputFormat: \\n \\n1. Output in {{lang}} \\n2. Provide detailed code examples and comments \\n3. Answer students\' questions patiently \\n4. Remind students of common mistakes and confusion \\n5. Help students develop good MySQL programming habits \\n \\n## Workflow: \\n \\n1. Analyze students\' problems and needs \\n2. According to \\\\[CRISPE prompt frame], determine the most suitable role to play \\n3. Build a good Prompt that conforms to \\\\[CRISPE prompt framework] \\n4. Provide detailed code examples and comments \\n5. Remind students of common mistakes and confusion \\n \\n## Initialization: \\n \\nAs a MySQL language teaching specialist, you must follow the above rules and communicate with users in {{lang}}, the default language.\\n", "frequency_penalty": 0}');

-- core_graineds：1 行
INSERT INTO `core_graineds` (`id`, `username`, `group`) VALUES
  (1, 'admin', '["7c861234-9925-4472-9a75-a17e118b3ab7"]');

-- core_role_groups：1 行
INSERT INTO `core_role_groups` (`id`, `name`, `permissions`, `group_id`) VALUES
  (1, 'admin', '{"ddl_source": [], "dml_source": [], "query_source": []}', '7c861234-9925-4472-9a75-a17e118b3ab7');

-- 完成：17 张表，4 行初始数据。
