<template>
  <a-modal
    v-model:open="is_open"
    :title="$t('db.edit.title')"
    width="1040px"
    :ok-text="$t('common.save')"
    :confirm-loading="saving"
    :cancel-button-props="{ disabled: saving }"
    :closable="!saving"
    :mask-closable="!saving"
    :keyboard="!saving"
    @ok="editDB"
  >
    <div class="datasource-edit-layout">
      <a-form
        name="datasource-edit"
        :model="dbForm"
        layout="vertical"
        class="datasource-edit-form"
      >
        <section class="edit-section">
          <h3>{{ $t('db.section.basic') }}</h3>
          <div class="edit-fields">
            <a-form-item :label="$t('common.table.env')" name="idc">
              <a-select v-model:value="dbForm.idc">
                <a-select-option v-for="i in idc" :key="i" :value="i"
                  >{{ i }}
                </a-select-option>
              </a-select>
            </a-form-item>
            <a-form-item :label="$t('common.table.name')" name="source">
              <a-input v-model:value="dbForm.source"></a-input>
            </a-form-item>
            <a-form-item :label="$t('common.table.db_type')">
              {{ dbTypeLabel(dbForm.db_type) }}
            </a-form-item>
            <a-form-item
              :label="$t('db.environment.tag')"
              name="environment_tag"
            >
              <a-auto-complete
                v-model:value="dbForm.environment_tag"
                style="width: 100%"
                :options="productionTagOptions"
                :placeholder="$t('db.environment.tag.tips')"
              />
            </a-form-item>
          </div>
        </section>
        <section class="edit-section">
          <h3>{{ $t('db.section.connection') }}</h3>
          <div class="edit-fields">
            <a-form-item
              class="wide-field"
              :label="$t('db.addr')"
              name="ip"
              :extra="
                dbForm.db_type === DBType.MongoDB
                  ? $t('db.mongo.hostsTips')
                  : undefined
              "
            >
              <a-input
                v-model:value="dbForm.ip"
                :placeholder="
                  dbForm.db_type === DBType.MongoDB
                    ? $t('db.mongo.hostsPlaceholder')
                    : undefined
                "
              ></a-input>
            </a-form-item>
            <a-form-item :label="$t('db.port')" name="port">
              <a-input-number v-model:value="dbForm.port"></a-input-number>
            </a-form-item>
            <a-form-item :label="$t('user.username')" name="username">
              <a-input v-model:value="dbForm.username"></a-input>
            </a-form-item>
            <a-form-item :label="$t('common.password')" name="password">
              <a-input-password
                v-model:value="dbForm.password"
                :disabled="clearPassword"
                :placeholder="$t('db.edit.passwordTips')"
              ></a-input-password>
            </a-form-item>
            <a-form-item
              class="wide-field"
              v-if="dbForm.db_type === DBType.MongoDB"
            >
              <a-checkbox v-model:checked="clearPassword">
                {{ $t('db.mongo.clearPassword') }}
              </a-checkbox>
            </a-form-item>
            <a-form-item
              v-if="dbForm.db_type === DBType.MongoDB"
              :label="$t('db.authDatabase')"
              name="auth_database"
            >
              <a-input
                v-model:value="dbForm.auth_database"
                :placeholder="$t('db.authDatabaseTips')"
              />
            </a-form-item>
            <a-form-item
              v-if="dbForm.db_type === DBType.MongoDB"
              :label="$t('db.mongo.replicaSet')"
              name="mongo_replica_set"
            >
              <a-input
                v-model:value="dbForm.mongo_replica_set"
                :placeholder="$t('db.mongo.replicaSetTips')"
              />
            </a-form-item>
            <a-form-item :label="$t('db.kind')">
              <a-switch
                v-model:checked="checked"
                :checked-children="$t('db.ssl')"
                :un-checked-children="$t('user.password')"
              />
            </a-form-item>
            <a-form-item
              v-if="dbForm.db_type === DBType.MongoDB"
              :label="$t('db.mongo.authMechanism')"
              name="mongo_auth_mechanism"
            >
              <a-select v-model:value="dbForm.mongo_auth_mechanism">
                <a-select-option value="">{{
                  $t('db.mongo.authAuto')
                }}</a-select-option>
                <a-select-option value="SCRAM-SHA-1"
                  >SCRAM-SHA-1</a-select-option
                >
                <a-select-option value="SCRAM-SHA-256"
                  >SCRAM-SHA-256</a-select-option
                >
              </a-select>
            </a-form-item>
            <template v-if="checked">
              <a-form-item v-if="dbForm.db_type === DBType.MongoDB">
                {{ $t('db.mongo.tlsTips') }}
              </a-form-item>
              <a-form-item
                class="wide-field"
                :label="$t('db.ssl.ca')"
                name="ca_file"
              >
                <a-textarea
                  v-model:value="dbForm.ca_file"
                  :rows="5"
                ></a-textarea>
              </a-form-item>
              <a-form-item
                class="wide-field"
                :label="$t('db.ssl.certificate')"
                name="cert"
              >
                <a-textarea v-model:value="dbForm.cert" :rows="5"></a-textarea>
              </a-form-item>
              <a-form-item
                class="wide-field"
                :label="$t('db.ssl.key')"
                name="key_file"
              >
                <a-textarea
                  v-model:value="dbForm.key_file"
                  :rows="5"
                ></a-textarea>
              </a-form-item>
            </template>
          </div>
        </section>
        <section class="edit-section">
          <h3>{{ $t('db.section.access') }}</h3>
          <div class="edit-fields">
            <a-form-item :label="$t('common.table.type')" name="is_query">
              <a-radio-group v-model:value="dbForm.is_query" name="radioGroup">
                <a-radio :value="2">{{ $t('db.rw') }}</a-radio>
                <a-radio :value="0">{{ $t('db.w') }}</a-radio>
                <a-radio :value="1">{{ $t('db.r') }}</a-radio>
              </a-radio-group>
            </a-form-item>
            <a-form-item :label="$t('menu.manage.flow')" name="flow_id">
              <a-select v-model:value="dbForm.flow_id" @change="mergeFlow">
                <a-select-option v-for="i in flow" :key="i.id" :value="i.id"
                  >{{ i.source }}
                </a-select-option>
              </a-select>
            </a-form-item>
            <a-form-item :label="$t('db.principal')" name="principal">
              <a-select
                v-model:value="dbForm.principal"
                style="width: 100%"
                show-search
                :options="
                  principalList.map((item) => ({
                    label: item.username,
                    value: item.username,
                  }))
                "
              >
              </a-select>
            </a-form-item>
            <a-form-item :label="$t('db.excluded.schema')">
              <a-select
                v-model:value="excludeDB"
                mode="multiple"
                show-search
                style="width: 100%"
                :max-tag-count="6"
              >
                <a-select-option v-for="i in schemaList" :key="i" :value="i"
                  >{{ i }}
                </a-select-option>
              </a-select>
            </a-form-item>
            <a-form-item :label="$t('db.excluded.field')">
              <a-select
                v-model:value="insulateWord"
                mode="tags"
                style="width: 100%"
                placeholder="Tags Mode"
                :max-tag-count="6"
              ></a-select>
            </a-form-item>
            <a-form-item :label="$t('common.rules')" name="rule_id">
              <a-select v-model:value="dbForm.rule_id" style="width: 100%">
                <a-select-option :key="-1" :value="0">{{
                  $t('db.global')
                }}</a-select-option>
                <a-select-option
                  v-for="i in compatibleRules"
                  :key="i.id"
                  :value="i.id"
                  >{{ i.desc }}
                </a-select-option>
              </a-select>
            </a-form-item>
            <!-- Redis：诊断命令（SLOWLOG/LATENCY/DUMP/COMMAND/INFO 全量）默认不放行，
               它们能看到实例配置或其它业务的执行信息 -->
            <a-form-item
              v-if="dbForm.db_type === DBType.Redis"
              :label="$t('db.allow.diag')"
            >
              <a-switch v-model:checked="dbForm.allow_diag_commands" />
              <div class="field-help">
                {{ $t('db.allow.diag.tips') }}
              </div>
            </a-form-item>
            <!-- 生产标记：审核侧对生产写入给出「补影响面/回滚」提示 -->
            <a-form-item :label="$t('db.is.production')" name="is_production">
              <a-switch v-model:checked="dbForm.is_production" />
            </a-form-item>
            <!-- Redis 部署形态：自动探测靠 CLUSTER INFO，部分云代理判不准，允许手动指定 -->
            <a-form-item
              v-if="dbForm.db_type === DBType.Redis"
              :label="$t('db.redis.mode')"
            >
              <a-select v-model:value="dbForm.redis_mode" style="width: 100%">
                <a-select-option value="">自动探测</a-select-option>
                <a-select-option value="standalone">standalone</a-select-option>
                <a-select-option value="cluster">cluster</a-select-option>
                <a-select-option value="proxy">proxy</a-select-option>
              </a-select>
              <div class="field-help">
                {{ $t('db.redis.mode.tips') }}
              </div>
            </a-form-item>
          </div>
        </section>
        <a-form-item :label="$t('common.action')">
          <a-button type="primary" ghost :loading="loading" @click="checkConn"
            >{{ $t('db.test') }}
          </a-button>
        </a-form-item>
      </a-form>
      <aside class="approval-preview" :aria-label="$t('db.section.approval')">
        <h3>{{ $t('db.section.approval') }}</h3>
        <a-steps
          v-if="steps.length"
          direction="vertical"
          size="small"
          :current="0"
        >
          <a-step v-for="i in steps" :key="i.desc" :title="i.desc">
            <template #description>
              <a-tag v-for="k in i.auditor" :key="k" color="blue">{{
                k
              }}</a-tag>
            </template>
          </a-step>
        </a-steps>
        <p v-else>{{ $t('db.flow.empty') }}</p>
      </aside>
    </div>
  </a-modal>
</template>

<script lang="ts" setup>
  import { getFlowProfile, Steps } from '@/apis/flow';
  import CommonMixins from '@/mixins/common';
  import { ref, computed, onMounted } from 'vue';
  import { useCommonStore } from '@/store';
  import { createSource, dbTypeLabel, Source, DBType } from '@/apis/db';
  import { querySchemaList } from '@/apis/source';
  import { CustomRule, isRuleSetCompatibleWithDBType } from '@/apis/rules';
  import { getSettingInfo } from '@/apis/setting';

  const props = defineProps<{
    rules: CustomRule[];
  }>();

  // 编辑保存成功后通知父组件(刷新列表),替代原全局 EventBus('postOk')
  const emit = defineEmits<{ edited: [] }>();

  const { is_open, turnState } = CommonMixins();

  const clearPassword = ref(false);
  const sqlTLS = ref(false);
  const checked = computed({
    get: () =>
      dbForm.value.db_type === DBType.MongoDB
        ? !!dbForm.value.mongo_tls
        : sqlTLS.value,
    set: (value: boolean) => {
      if (dbForm.value.db_type === DBType.MongoDB)
        dbForm.value.mongo_tls = value;
      else sqlTLS.value = value;
    },
  });

  const commonStore = useCommonStore();

  const loading = ref(false);
  const saving = ref(false);

  const idc = computed(() => {
    return commonStore.idc;
  });

  const flow = computed(() => {
    return commonStore.flow;
  });

  const steps = ref([] as Steps[]);

  const principalList = computed(() => commonStore.principal);

  const productionTags = ref<string[]>([]);

  const productionTagOptions = computed(() =>
    productionTags.value.map((tag) => ({ value: tag })),
  );

  const compatibleRules = computed(() =>
    (props.rules || []).filter((item) =>
      isRuleSetCompatibleWithDBType(item, dbForm.value.db_type),
    ),
  );

  onMounted(async () => {
    const { data } = await getSettingInfo();
    productionTags.value = Array.isArray(
      data.payload.other?.production_audit_tags,
    )
      ? data.payload.other.production_audit_tags
      : ['production', 'pre-production'];
  });

  let dbForm = ref({
    idc: '',
    source: '',
    ip: '',
    port: 0,
    password: '',
    username: '',
    auth_database: '',
    mongo_replica_set: '',
    mongo_tls: false,
    mongo_auth_mechanism: '',
    is_query: 2,
    flow_id: 0,
    source_id: '',
    exclude_db_list: '',
    insulate_word_list: '',
    rule_id: 0,
    environment_tag: '',
    allow_diag_commands: false,
    redis_mode: '',
    is_production: false,
  } as Source);

  const schemaList = ref<string[]>([]);

  const excludeDB = ref<string[]>([]);

  const insulateWord = ref<string[]>([]);

  const mergeFlow = async (vl: number) => {
    const { data } = await getFlowProfile(vl);
    steps.value = Array.isArray(data.payload?.steps) ? data.payload.steps : [];
  };

  const editDB = async () => {
    if (saving.value) return;
    saving.value = true;
    try {
      dbForm.value.exclude_db_list = excludeDB.value.join(',');
      dbForm.value.insulate_word_list = insulateWord.value.join(',');
      await createSource({
        db: {
          ...dbForm.value,
          password: clearPassword.value ? '' : dbForm.value.password,
        },
        tp: 'edit',
        clear_password: clearPassword.value,
      });
      turnState();
      emit('edited');
    } finally {
      saving.value = false;
    }
  };

  const fetchSchema = async () => {
    schemaList.value = [];
    try {
      const { data } = await querySchemaList(dbForm.value.source_id);
      schemaList.value = Array.isArray(data.payload) ? data.payload : [];
    } catch {
      // 请求层已提示连接错误，仍允许编辑已有的连接配置。
    }
  };

  const fillInfo = async (vl: any) => {
    dbForm.value = Object.assign(
      { password: '', mongo_tls: false, mongo_auth_mechanism: '' },
      vl,
    );
    clearPassword.value = false;
    turnState();
    excludeDB.value = dbForm.value.exclude_db_list.split(',');
    insulateWord.value = dbForm.value.insulate_word_list
      .split(',')
      .map((i: string) => i.trim())
      .filter(Boolean);
    fetchSchema();
    const { data } = await getFlowProfile(dbForm.value.flow_id);
    steps.value = Array.isArray(data.payload?.steps) ? data.payload.steps : [];
  };

  const checkConn = async () => {
    if (loading.value) return;
    loading.value = true;
    try {
      await createSource({
        db: {
          ...dbForm.value,
          password: clearPassword.value ? '' : dbForm.value.password,
        },
        tp: 'test',
        clear_password: clearPassword.value,
      });
    } finally {
      loading.value = false;
    }
  };

  defineExpose({
    fillInfo,
  });
</script>

<style scoped>
  .datasource-edit-layout {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 220px;
    gap: 24px;
    max-height: min(70vh, 760px);
    overflow-y: auto;
    padding: 8px 4px 0 0;
  }
  .datasource-edit-form {
    min-width: 0;
  }
  .edit-section {
    margin-bottom: 24px;
  }
  .edit-section h3,
  .approval-preview h3 {
    margin: 0 0 18px;
    font-size: 14px;
    font-weight: 600;
  }
  .edit-section + .edit-section {
    padding-top: 20px;
    border-top: 1px solid var(--frame-line);
  }
  .edit-fields {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    column-gap: 20px;
  }
  .wide-field {
    grid-column: 1 / -1;
  }
  :deep(.ant-form-item) {
    min-width: 0;
    margin-bottom: 18px;
  }
  :deep(.ant-input-number) {
    width: 100%;
  }
  :deep(.ant-form-item-extra),
  .field-help {
    color: var(--text-muted);
    font-size: 12px;
    line-height: 1.6;
    overflow-wrap: anywhere;
  }
  .field-help {
    margin-top: 6px;
  }
  .approval-preview {
    align-self: start;
    padding: 18px;
    border: 1px solid var(--frame-line);
    border-radius: 10px;
    background: var(--surface-muted);
    overflow-wrap: anywhere;
  }
  .approval-preview p {
    margin: 0;
    color: var(--text-muted);
    font-size: 13px;
  }
  @media (max-width: 767px) {
    .datasource-edit-layout {
      grid-template-columns: minmax(0, 1fr);
      gap: 16px;
    }
    .edit-fields {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
