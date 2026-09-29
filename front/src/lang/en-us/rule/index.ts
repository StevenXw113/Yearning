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
  ruleSearchTips: 'Please enter a rule description to search',
  upstreamCheck: 'Check upstream',
  upstreamLatest: 'Already up to date ({version})',
  upstreamNew: 'New upstream version',
  upstreamNewDesc:
    'Current {current}, upstream latest {latest}. Updating happens in the source tree: run ./scripts/upgrade-engine.sh inside engine/ (syncs upstream and regenerates the rule list; add --preview to dry-run first), then redeploy the engine (see engine/README.md). This page only detects.',
  ruleHistory: 'Change history',
  ruleHelp: 'How to use',
  ruleDeleteConfirm: 'Delete this rule set? (refused if a data source still uses it)',
  ruleDeleteDone: 'Rule set deleted — you can restore it from the change history',
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
  ruleRollbackConfirm:
    'Roll back to this version? Current rules will be overwritten (the rollback itself is recorded too).',
  ruleRollbackDone: 'Rolled back',
  MongoForbidEmptyFilter: 'MongoDB: forbid update / delete without filter',
  MongoForbidDangerous: 'MongoDB: forbid dropDatabase / eval / mapReduce',
  MongoForbidWhere: 'MongoDB: forbid $where (server-side script)',
  MongoForbidDropCollection: 'MongoDB: forbid drop / renameCollection',
  MongoForbidSystemCollection: 'MongoDB: forbid changes on system.* collections',
  MongoForbidAdminCommand: 'MongoDB: forbid user/role management commands (createUser, ...)',
  MongoForbidImmutableId: 'MongoDB: forbid modifying _id (immutable field)',
  MongoForbidCappedConvert: 'MongoDB: forbid convertToCapped (rebuilds the collection)',
  MongoForbidCollMod: 'MongoDB: forbid collMod (validator / index options)',
  MongoIndexKeyLimit: 'MongoDB: max keys per index (0 = unlimited, recommended 5)',
  MongoIndexNameSpec: 'MongoDB: index name pattern (regex, empty = off, e.g. ^idx_)',
  MongoCollectionPrefix: 'MongoDB: collection name prefix (empty = off)',
  MongoMaxCollectionNameLen: 'MongoDB: max collection name length (0 = unlimited, e.g. 120)',
  MongoRegexUnanchored: 'MongoDB: forbid $regex without ^ anchor (cannot use index)',
  MongoNegationOperator: 'MongoDB: discourage $ne / $nin / $not / $nor (usually a full scan)',
  MongoOrClause: 'MongoDB: discourage $or (each branch needs its own index)',
  MongoLargeInList: 'MongoDB: max elements in $in (0 = unlimited, recommended 1000)',
  MongoQueryForbidNoFilter: 'MongoDB query: no filter allowed (find without filter / aggregate without $match)',
  MongoQueryForbidLookup: 'MongoDB query: forbid $lookup / $graphLookup',
  MongoQueryForbidNoLimit: 'MongoDB query: find with sort must set limit',
  MongoMaxAffectRows: 'MongoDB: max documents affected per change (0 = unlimited)',
};
