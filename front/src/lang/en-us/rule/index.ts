export default {
  DMLTransaction: 'DML statements are executed using a transaction',
  DDLCheckTableComment: 'Tables must have table comment',
  DDlCheckColumnComment: 'Table fields must have column comments',
  DDLCheckColumnNullable: 'Fields of non-TIMESTAMP type must be NOT NULL',
  DDLCheckColumnDefault:
    'Non-text,blob, JSON, TIMESTAMP fields must have default values',
  DDLCheckFloatDouble: 'Force the float/double type to be of type Decimal',
  DDLEnableAutoincrementInit: 'The increment column to start with 1',
  DDLPrimaryKeyMust: 'Force the primary key name to be ID',
  DDLEnableAutoIncrement: 'Forces primary keys to increment columns',
  DDLEnableAutoincrementUnsigned:
    'The primary key must use the unsigned flag unsigned',
  DDLIndexNameSpec:
    'Index name specification (index names must begin with idx_)',
  CheckIdentifier: 'Enable Mysql keyword Check',
  DDLEnableAcrossDBRename: 'Allow migration across tables',
  DDLEnableDropTable: 'Table deletion is allowed',
  DDLEnableDropDatabase: 'Allow database deletion',
  DDLForbidTruncate: 'Forbid TRUNCATE statement (custom rule example)',
  DMLInsertMustExplicitly: 'Require explicit column list in INSERT',
  DDLEnableForeignKey: 'Allow foreign keys',
  DDLTablePrefix: 'Table name prefix (e.g. app_)',
  DDLAllowPRINotInt: 'Non-int /bigint primary key type is allowed',
  DDLEnableNullIndexName: 'Empty index name is allowed',
  DDLMultiToCommit:
    'Allows a single work order to submit multiple DDL statements',

  DDLAllowMultiAlter:
    'Allows a single work order to execute multiple ALTER statements',
  DDLAllowColumnType:
    'Allows fields to be typed (converted between different fields or changed from long to short in length). For example :int -> bigint,int(50) -> int(20))',
  DDLAllowChangeColumnPosition: 'After /first is allowed',
  AllowCreateView: 'Allow creation of views',
  AllowCreatePartition: 'Allow partition creation',
  AllowSpecialType: 'Bit,enum, and SET fields are allowed',
  SupportCollation:
    'The Collate range allowed when CREATE/ALTER a table or field. Use commas to separate multiple items',
  SupportCharset:
    'The RANGE OF CHARACTER sets ALLOWED WHEN CREATE/ALTER A TABLE or FIELD. Use commas to separate multiple items',
  MustHaveColumns:
    'Table must have fields. Separate multiple fields with commas',
  DDLMaxKeyParts: 'A single index specifies the upper limit of a field',
  DDLMaxKey: 'A single table allows a maximum of several indexes',
  MaxDDLAffectRows: 'Maximum number of rows affected by DDL',
  DDLMaxCharLength: 'Char Indicates the maximum length of the char field',
  MaxTableNameLen: 'Maximum length of a table name',
  DMLMaxInsertRows: 'Insert Indicates the maximum number of rows inserted',
  DMLAllowLimitSTMT:
    'The limit keyword is allowed for UPDATE/INSERT statements',
  DMLAllowInsertNull: 'Insert statements are allowed to insert null values',
  DDLImplicitTypeConversion: 'Implicit conversions are not allowed',
  DMLInsertColumns:
    'Check whether the field name inserted in the Insert statement exists',
  DMLWhere: 'Enforce that DML statements must have a WHERE condition',
  DMLOrder: 'Disallow DML statements from using the Order BY clause',
  DMLSelect: 'Disallow Select clauses for DML statements',
  DMLWhereExprValueIsNull:
    'In a WHERE clause, it is not allowed to compare NULL values with other fields or values.',
  MaxAffectRows: 'DML affects the maximum number of rows',
  IsOSC: 'Start the online table change tool',
  OscSize:
    'When the volume of a table exceeds the value, online synchronization of table changes is triggered. The unit is M',
  OSCExpr:
    'Synchronize tool parameters. For example: PT-OSC please note! Set only parameters here. Replace the input value $SQL $ADDR $PORT $USER $PASSWORD $SCHEMA $TABLE with the following variable name. Example: (PT-OSC configuration) pt-online-schema-change  --alter $SQL --user=$USER  --password=$PASSWORD  --host=$ADDR P=$PORT,D=$SCHEMA,t=$TABLE  --execute',
  global: 'Global rules',
  custom_list: 'Custom rule list',
  custom: 'Custom rule',
  customRule: 'Custom audit rules',
  customRuleHelp:
    'Built-in rules are compiled into the engine. Add your own regex checks here: submitted SQL is matched as text and blocked or flagged by level. No syntax parsing.',
  customRuleAdd: 'New rule',
  customRuleEdit: 'Edit rule',
  customRuleName: 'Rule name',
  customRulePattern: 'Regular expression',
  customRuleDesc: 'Description',
  customRuleLevel: 'Level',
  customRuleLevelError: 'Block',
  customRuleLevelWarn: 'Warn',
  customRuleEnabled: 'Enabled',
  customRuleDeleteConfirm: 'Delete this custom rule?',
  customRuleSaved: 'Custom rule saved',
  customRuleDeleted: 'Custom rule deleted',

  ruleSearchTips: 'Please enter a rule description to search',
  upstreamCheck: 'Check upstream',
  upstreamLatest: 'Already up to date ({version})',
  upstreamNew: 'New upstream version',
  upstreamNewDesc:
    'Current {current}, upstream latest {latest}. Updating happens in the source tree: run ./scripts/upgrade-engine.sh inside engine/ (syncs upstream and regenerates the rule list; add --preview to dry-run first), then redeploy the engine (see engine/README.md). This page only detects.',
  ruleHistory: 'Change history',
  ruleHelp: 'How to use',
  ruleDeleteConfirm:
    'Delete this rule set? (refused if a data source still uses it)',
  ruleDeleteDone:
    'Rule set deleted — you can restore it from the change history',
  ruleHelpIntro:
    'Every row on this page (including custom rules that default to off) is defined in engine code; the page only controls the switch and the level. Data sources bind a rule set and configure masked fields in Data source management.',
  ruleHelpAdd:
    'Add: copy engine/internal/customrules/forbid_truncate.go inside engine/internal/customrules/ (implement Name/Check + Register in init). If it needs a switch, follow the four-place wiring in that directory README — missing the convert.go mapping makes the switch silently ineffective.',
  ruleHelpEnable:
    'Enable: tick the switch and save. Set the level to "observe" or "notify" first and watch for false positives before promoting it to "block".',
  ruleHelpDelete:
    'Delete: to merely disable it, untick the switch (or set the level to observe). To remove it for good, delete the rule file and its switch wiring (proto / app / frontend). Rule sets themselves can be deleted in the Custom rules tab; deletion is refused while a data source uses it.',
  ruleHelpDoc:
    'Full details: engine/internal/customrules/README.md and the "把上游规则接到开关上" section in engine/README.md.',
  ruleOperator: 'Operator',
  ruleCreatedAt: 'Time',
  ruleLevelTitle: 'Level',
  ruleLevelError: 'Block (error)',
  ruleLevelWarn: 'Notify (warning)',
  ruleLevelObserve: 'Observe (no block)',
  ruleRollback: 'Rollback',
  ruleLevelDefault: 'Default',
  ruleLevelShortError: 'Block',
  ruleLevelShortWarn: 'Notify',
  ruleLevelShortObserve: 'Observe',
  ruleRollbackConfirm:
    'Roll back to this version? Current rules will be overwritten (the rollback itself is recorded too).',
  ruleRollbackDone: 'Rolled back',
  MongoForbidEmptyFilter: 'MongoDB: forbid update / delete without filter',
  MongoForbidDangerous: 'MongoDB: forbid dropDatabase / eval / mapReduce',
  MongoForbidWhere: 'MongoDB: forbid $where (server-side script)',
  MongoForbidDropCollection: 'MongoDB: forbid drop / renameCollection',
  MongoForbidSystemCollection:
    'MongoDB: forbid changes on system.* collections',
  MongoForbidAdminCommand:
    'MongoDB: forbid user/role management commands (createUser, ...)',
  MongoForbidImmutableId: 'MongoDB: forbid modifying _id (immutable field)',
  MongoForbidCappedConvert:
    'MongoDB: forbid convertToCapped (rebuilds the collection)',
  MongoForbidCollMod: 'MongoDB: forbid collMod (validator / index options)',
  MongoIndexKeyLimit:
    'MongoDB: max keys per index (0 = unlimited, recommended 5)',
  MongoIndexNameSpec:
    'MongoDB: index name pattern (regex, empty = off, e.g. ^idx_)',
  MongoCollectionPrefix: 'MongoDB: collection name prefix (empty = off)',
  MongoMaxCollectionNameLen:
    'MongoDB: max collection name length (0 = unlimited, e.g. 120)',
  MongoRegexUnanchored:
    'MongoDB: forbid $regex without ^ anchor (cannot use index)',
  MongoNegationOperator:
    'MongoDB: discourage $ne / $nin / $not / $nor (usually a full scan)',
  MongoOrClause: 'MongoDB: discourage $or (each branch needs its own index)',
  MongoLargeInList:
    'MongoDB: max elements in $in (0 = unlimited, recommended 1000)',
  MongoQueryForbidNoFilter:
    'MongoDB query: no filter allowed (find without filter / aggregate without $match)',
  MongoQueryForbidLookup: 'MongoDB query: forbid $lookup / $graphLookup',
  MongoQueryForbidNoLimit: 'MongoDB query: find with sort must set limit',
  MongoMaxAffectRows:
    'MongoDB: max documents affected per change (0 = unlimited)',
  RedisCommandSyntax:
    'Check command, subcommand, arity and option combinations; block invalid syntax',
  RedisUnknownCommand:
    'Forbid commands without registered semantics, module and vendor commands',
  RedisKeyExtraction:
    'Both source and target keys must be resolved; block when they cannot be',
  RedisKeyPrefix:
    'Keys must stay within the allowed prefix; source and target keys are both checked; empty = off',
  RedisMaxKeyLen: 'Max key name length in bytes, 0 = unlimited',
  RedisForbiddenKeyPattern:
    'Forbid protected keys (system keys, core locks, internal state); multiple patterns separated by commas',
  RedisSensitiveKeyAccess:
    'Warn when accessing sensitive keys (credentials, tokens, personal data); patterns configured by business',
  RedisDbScope:
    'Commands may only touch the allowed DB range (e.g. 0-15); block outside it',
  RedisForbidConnectionControl:
    'Forbid AUTH / HELLO and other connection-state commands; the entry point manages connections',
  RedisForbidDangerous:
    'Forbid FLUSHALL / FLUSHDB / SHUTDOWN / SWAPDB (instance-wide operations)',
  RedisForbidAdmin:
    'Forbid CONFIG SET/REWRITE/RESETSTAT and CLIENT KILL/PAUSE; read-only subcommands are allowed',
  RedisForbidDebug: 'Forbid DEBUG',
  RedisForbidMonitor: 'Forbid MONITOR (performance impact and data exposure)',
  RedisForbidAclChange: 'Forbid ACL change subcommands such as SETUSER / DELUSER / LOAD',
  RedisForbidModuleChange: 'Forbid MODULE LOAD / LOADEX / UNLOAD',
  RedisForbidReplica: 'Forbid REPLICAOF / SLAVEOF (changes replication topology)',
  RedisForbidClusterChange:
    'Forbid CLUSTER subcommands that change topology or slots; read-only ones are allowed',
  RedisForbidPersistence: 'Forbid SAVE / BGSAVE / BGREWRITEAOF from the business entry point',
  RedisForbidMigration: 'Forbid MIGRATE / RESTORE; require a dedicated review',
  RedisForbidMove: 'Forbid MOVE; SELECT is checked by the DB scope rule',
  RedisForbidKeysCommand: 'Forbid KEYS, use a bounded SCAN instead',
  RedisForbidRandomKey: 'Forbid RANDOMKEY (result is outside the authorized key range)',
  RedisScanMatchRequired:
    'SCAN must carry an allowed MATCH range; block when it cannot be confirmed',
  RedisMaxScanCount: 'Max COUNT for SCAN / HSCAN / SSCAN / ZSCAN, 0 = no check',
  RedisFullCollectionRead:
    'Warn on HGETALL / SMEMBERS / full LRANGE / ZRANGE when collection size is unknown or large',
  RedisMaxReadElements: 'Max elements per range or random read, 0 = unlimited',
  RedisMaxReadKeys: 'Max keys per batch read command such as MGET, 0 = unlimited',
  RedisExpensiveRead:
    'Warn on SORT and set intersections/unions when input size is unknown or over budget',
  RedisMaxValueSize:
    'Max bytes of a single written value, 0 = unlimited; block when exceeded',
  RedisMaxCommandPayload:
    'Max total payload bytes per command, 0 = unlimited; prevents bypassing the value limit',
  RedisMaxWriteKeys:
    'Max keys per write command, 0 = unlimited; over the limit requires batch review',
  RedisMaxWriteElements: 'Max fields / members / elements written per command, 0 = unlimited',
  RedisNonIdempotentWrite: 'Warn on non-idempotent writes such as INCR / DECR / list pop',
  RedisLargeOffsetWrite:
    'Max projected object size for offset writes (SETRANGE / SETBIT), 0 = unlimited',
  RedisForbidRename:
    'Forbid RENAME / RENAMENX; both change key ownership, RENAME may overwrite',
  RedisCopyOverwrite:
    'Warn on COPY ... REPLACE and check source, target and target DB permissions',
  RedisStoreOperation:
    'Check input size, target overwrite and key permissions for store operations',
  RedisRequireTtl: 'Cache keys must carry a TTL; registered permanent keys may be exempted',
  RedisTtlLossOnOverwrite:
    'Warn when an overwrite may drop the existing TTL without KEEPTTL or a new expiry',
  RedisRequireTtlExempt:
    'Exempt prefixes (comma separated): permanent business keys skip the TTL requirement',
  RedisForbidPersist: 'Forbid PERSIST; permanent business keys may be exempted',
  RedisMinTtl: 'Warn when the TTL is below the business minimum; 0 = no check',
  RedisMaxTtl: 'Warn when the TTL exceeds the business maximum; 0 = no check',
  RedisImmediateExpire: 'Warn when the expiry is immediate or already past (the key is deleted)',
  RedisDeleteKey: 'Warn on DEL / UNLINK data deletion',
  RedisMaxDeleteKeys:
    'Max keys per delete command, 0 = unlimited; over the limit requires batch review',
  RedisProtectedKeyDelete:
    'Forbid deleting protected keys (locks, idempotency keys, core state); comma separated patterns',
  RedisLargeRangeDelete:
    'Warn on range deletes, list trims and stream trims when the affected count is unknown',
  RedisForbidScript:
    'Forbid unregistered EVAL / EVALSHA / EVAL_RO / EVALSHA_RO / FCALL / FCALL_RO',
  RedisForbidScriptManagement:
    'Forbid SCRIPT / FUNCTION subcommands that change script or function state; read-only ones allowed',
  RedisForbidWatch: 'Forbid WATCH / UNWATCH unless full session semantics are supported',
  RedisForbidBlockingCommand:
    'Forbid blocking commands (list / stream waits); configure dedicated scenarios separately',
  RedisForbidSubscription:
    'Forbid SUBSCRIBE / PSUBSCRIBE / SSUBSCRIBE and other long-lived subscriptions',
  RedisStreamStateChange: 'Warn on consumer-group changes, acknowledgements and claims',
  RedisTransactionIntegrity:
    'Block MULTI/EXEC: single-command entry cannot guarantee every command in the transaction is reviewed',
  RedisClusterDbZero:
    'Native Cluster only supports db0; block other logical DBs',
  RedisClusterCrossSlot:
    'In Cluster, multi-key commands must hash to the same slot',
  RedisClusterScanScope:
    'In Cluster, SCAN only covers one node and cannot serve as a global scan',
  RedisClusterTransactionSlot:
    'Cluster transactions require all keys in one slot; block when unverifiable',
  RedisClusterScriptSlot:
    'Cluster scripts require all keys in one slot; block when key declaration is missing',
  RedisProxyDbSupport:
    'Proxy mode: block non-db0 logical DBs whose support is unverified',
  RedisProxyCrossShard:
    'Proxy mode: block multi-key commands whose sharding behaviour is unverified',
  RedisProxyScanScope:
    'Proxy mode: block scans when coverage and cursor semantics are unknown',

  RedisProxyCommandSupport:
    'Commands not implemented or restricted on Proxy cluster (e.g. BZPOPMAX/BZPOPMIN unsupported on Redis 4.0 proxy)',
  RedisProxyAtomicity:
    'Source-to-destination commands cannot stay atomic across shards (SMOVE/RENAME/RPOPLPUSH/BITOP/MSETNX need same slot)',
  RedisProxyScriptSupport:
    'Proxy cluster requires scripts to declare keys (numkeys >= 1) and all keys in the same slot',
  RedisProxyTransactionSupport:
    'Proxy cluster only supports single-shard transactions; MULTI/EXEC and WATCH across shards are unreliable',
  RedisReadConsistency:
    'Multi-key reads across shards are not a single point-in-time snapshot; some values may be stale',
  RedisTypeMismatch: 'Key type does not match what the write command expects; the command will fail with WRONGTYPE',
  RedisOverwriteExisting: 'Overwriting an existing key discards its previous value',
  RedisLargeValueRead: 'Reading a key larger than 1 MB holds the query connection for a long time',
  RedisBigKeySyncDelete: 'Synchronous deletion of a big key blocks the server; prefer UNLINK',
  RedisProductionWrite: 'Write against a production data source; describe blast radius and rollback',
  RedisCommandCompatibility: 'Command requires a newer Redis version than this instance provides',
  RedisBatchExpireConcentration:
    'Round-hour TTLs make similar keys expire at the same instant; add jitter',
  RedisDeleteManifestRequired:
    'Bulk deletion (10+ keys) requires a key manifest and confirmed backup',
  RedisMaxTransactionCommands:
    'Max commands per submission (0 = unlimited); split larger submissions',
  RedisScriptTemplateScope:
    'Scripts may only touch explicitly declared keys (no KEYS index overflow) within the allowed prefix',
  DorisRequireDistribution:
    'Doris: require DISTRIBUTED BY when creating table (random bucketing is uncontrolled)',
  DorisBucketLimit: 'Doris: max BUCKETS per table (0 = unlimited)',
  DorisReplicationNum: 'Doris: expected replication num (0 = no check)',
  DorisKeyModel:
    'Doris: warn on AGGREGATE KEY model (wrong aggregate type yields wrong results)',
  ruleCatAll: 'All',
  ruleCatStatement: 'Statement',
  ruleCatTable: 'Table',
  ruleCatColumn: 'Column',
  ruleCatIndex: 'Index',
  ruleCatNaming: 'Naming',
  ruleCatSystem: 'System',
  ruleCatEngine: 'Engine',
  ruleRefSources: 'Used by sources',
  ruleRefNone: '—',
};
