<template>
  <a-form
    ref="formRef"
    name="datasource-create"
    :model="dbForm"
    :rules="rules"
    layout="vertical"
    class="datasource-form"
  >
    <h3 class="form-section-title">{{ $t('db.section.basic') }}</h3>
    <a-form-item :label="$t('common.table.env')" name="idc">
      <a-select v-model:value="dbForm.idc">
        <a-select-option v-for="i in idc" :key="i" :value="i">{{
          i
        }}</a-select-option>
      </a-select>
    </a-form-item>
    <a-form-item :label="$t('common.table.db_type')" name="db_type">
      <a-select v-model:value="dbForm.db_type" @change="onTypeChange">
        <a-select-option
          v-for="i in DB_TYPES"
          :key="i.value"
          :value="i.value"
          >{{ i.label }}</a-select-option
        >
      </a-select>
    </a-form-item>
    <a-form-item :label="$t('db.environment.tag')" name="environment_tag">
      <a-auto-complete
        v-model:value="dbForm.environment_tag"
        style="width: 100%"
        :options="productionTagOptions"
        :placeholder="$t('db.environment.tag.tips')"
      />
    </a-form-item>
    <a-form-item :label="$t('common.rules')" name="rule_id">
      <!-- 新建时即可指定规则集,与编辑弹窗对齐;0 = 全局规则 -->
      <a-select v-model:value="dbForm.rule_id" style="width: 100%">
        <a-select-option :key="-1" :value="0">{{
          $t('db.global')
        }}</a-select-option>
        <a-select-option
          v-for="i in compatibleRuleSets"
          :key="i.id"
          :value="i.id"
          >{{ i.desc }}
        </a-select-option>
      </a-select>
    </a-form-item>
    <a-form-item :label="$t('common.table.name')" name="source">
      <a-input v-model:value="dbForm.source"></a-input>
    </a-form-item>
    <h3 class="form-section-title">{{ $t('db.section.connection') }}</h3>
    <a-form-item
      :label="$t('db.addr')"
      name="ip"
      :extra="
        dbForm.db_type === DBType.MongoDB ? $t('db.mongo.hostsTips') : undefined
      "
    >
      <a-input
        v-model:value="dbForm.ip"
        :placeholder="
          dbForm.db_type === DBType.MongoDB
            ? $t('db.mongo.hostsTips')
            : undefined
        "
      ></a-input>
    </a-form-item>
    <a-form-item :label="$t('db.port')" name="port">
      <a-input-number v-model:value="dbForm.port"></a-input-number>
    </a-form-item>
    <a-form-item :label="$t('db.kind')">
      <a-switch
        :aria-label="$t('db.kind')"
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
        <a-select-option value="SCRAM-SHA-1">SCRAM-SHA-1</a-select-option>
        <a-select-option value="SCRAM-SHA-256">SCRAM-SHA-256</a-select-option>
      </a-select>
    </a-form-item>
    <template v-if="checked">
      <a-form-item v-if="dbForm.db_type === DBType.MongoDB">
        {{ $t('db.mongo.tlsTips') }}
      </a-form-item>
      <a-form-item :label="$t('db.ssl.ca')" name="ca_file">
        <a-button size="small" @click="open.ca = true">
          {{ $t('db.upload') }}
        </a-button>
      </a-form-item>
      <a-form-item :label="$t('db.ssl.certificate')" name="cert">
        <a-button size="small" @click="open.cert = true">
          {{ $t('db.upload') }}
        </a-button>
      </a-form-item>
      <a-form-item :label="$t('db.ssl.key')" name="key_file">
        <a-button size="small" @click="open.key = true">
          {{ $t('db.upload') }}
        </a-button>
      </a-form-item>
    </template>
    <a-form-item :label="$t('user.username')" name="username">
      <a-input v-model:value="dbForm.username"></a-input>
    </a-form-item>
    <a-form-item :label="$t('user.password')" name="password">
      <a-input-password v-model:value="dbForm.password"></a-input-password>
    </a-form-item>
    <a-form-item
      v-if="dbForm.db_type === DBType.MongoDB"
      :label="$t('db.authDatabase')"
      name="auth_database"
    >
      <a-input
        v-model:value="dbForm.auth_database"
        :placeholder="$t('db.authDatabaseTips')"
      ></a-input>
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
    <h3 class="form-section-title">{{ $t('db.section.access') }}</h3>
    <a-form-item :label="$t('menu.manage.flow')" name="flow_id">
      <a-select v-model:value="dbForm.flow_id">
        <a-select-option v-for="i in flow" :key="i.id" :value="i.id">{{
          i.source
        }}</a-select-option>
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
    <a-form-item :label="$t('common.table.type')" name="is_query">
      <a-radio-group v-model:value="dbForm.is_query" name="radioGroup">
        <a-radio :value="2">{{ $t('db.rw') }}</a-radio>
        <a-radio :value="0">{{ $t('db.w') }}</a-radio>
        <a-radio :value="1">{{ $t('db.r') }}</a-radio>
      </a-radio-group>
    </a-form-item>
    <a-form-item :label="$t('common.action')">
      <a-space size="small" wrap>
        <a-button type="dashed" :loading="loading" @click="checkConn"
          >{{ $t('db.test') }}
        </a-button>
        <a-button type="primary" @click="createSource">{{
          $t('common.create')
        }}</a-button>
        <a-button @click="resetFields">{{ $t('db.form.reset') }} </a-button>
      </a-space>
    </a-form-item>
  </a-form>
  <ssl_model
    v-model:open="open.ca"
    title="CA"
    @save="
      (text: string) => {
        dbForm.ca_file = text;
        open.ca = false;
      }
    "
  ></ssl_model>
  <ssl_model
    v-model:open="open.cert"
    title="Cert"
    @save="
      (text: string) => {
        dbForm.cert = text;
        open.cert = false;
      }
    "
  ></ssl_model>
  <ssl_model
    v-model:open="open.key"
    title="KeyFile"
    @save="
      (text: string) => {
        dbForm.key_file = text;
        open.key = false;
      }
    "
  ></ssl_model>
</template>

<script lang="ts" setup>
  import { ref, computed, reactive, onMounted } from 'vue';
  import {
    DB_TYPES,
    dbTypePort,
    Source,
    createSource as cs,
    DBType,
  } from '@/apis/db';
  import {
    getCustomRulesList,
    isRuleSetCompatibleWithDBType,
    type CustomRule,
  } from '@/apis/rules';
  import { getSettingInfo } from '@/apis/setting';
  import { useCommonStore } from '@/store';
  import { RuleObject } from 'ant-design-vue/es/form';
  import { useI18n } from 'vue-i18n';
  import ssl_model from './sslModal.vue';

  const { t } = useI18n();

  // 新建数据源成功后通知父组件(由其刷新列表),替代原全局 EventBus('postOk')
  const emit = defineEmits<{ created: [] }>();

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

  // 规则集选项(与编辑弹窗同源 getCustomRulesList);0 = 全局规则
  const ruleSets = ref([] as CustomRule[]);
  const productionTags = ref<string[]>([]);

  const productionTagOptions = computed(() =>
    productionTags.value.map((tag) => ({ value: tag })),
  );

  const compatibleRuleSets = computed(() =>
    ruleSets.value.filter((item) =>
      isRuleSetCompatibleWithDBType(item, dbForm.value.db_type),
    ),
  );

  onMounted(async () => {
    const [rulesRes, settingRes] = await Promise.all([
      getCustomRulesList(),
      getSettingInfo(),
    ]);
    ruleSets.value = rulesRes.data.payload;
    productionTags.value = Array.isArray(
      settingRes.data.payload.other?.production_audit_tags,
    )
      ? settingRes.data.payload.other.production_audit_tags
      : ['production', 'pre-production'];
  });

  const open = reactive({
    ca: false,
    cert: false,
    key: false,
  });

  const commonStore = useCommonStore();

  const regExp_Name = (rule: RuleObject, value: string) => {
    let pPattern = new RegExp(
      "[`~!@#$^&*()={}':;',\\[\\]<>/?~！@#￥……&*（）——{}【】‘；：”“'。，、？]",
    );
    if (pPattern.test(value)) {
      return Promise.reject(t('db.exp.name'));
    } else {
      return Promise.resolve();
    }
  };

  const regExp_password = (rule: RuleObject, value: string) => {
    if (dbForm.value.db_type === DBType.MongoDB) return Promise.resolve();
    let pPattern = new RegExp('[`?？^$]');
    if (pPattern.test(value)) {
      return Promise.reject(t('db.exp.password'));
    } else {
      return Promise.resolve();
    }
  };

  const rules = computed(() => ({
    idc: [{ required: true, message: t('db.check.env'), trigger: 'change' }],
    source: [
      { required: true, message: t('db.check.name'), trigger: 'blur' },
      { validator: regExp_Name, trigger: 'blur' },
    ],
    ip: [{ required: true, message: t('db.check.addr'), trigger: 'blur' }],
    username: [
      {
        required:
          dbForm.value.db_type !== DBType.MongoDB || !!dbForm.value.password,
        message: t('db.check.username'),
        trigger: 'blur',
      },
    ],
    port: [
      {
        required: true,
        message: t('db.check.port'),
        trigger: 'blur',
        type: 'number',
      },
    ],
    flow_id: [
      { required: true, message: t('db.check.flow'), trigger: 'change' },
    ],
    principal: [
      { required: true, message: t('db.check.principal'), trigger: 'change' },
    ],
    password: [
      {
        required: dbForm.value.db_type !== DBType.MongoDB,
        message: t('db.check.password'),
        trigger: 'blur',
      },
      { validator: regExp_password, trigger: 'blur' },
    ],
  }));

  const loading = ref(false);

  const principalList = computed(() => commonStore.principal);

  const idc = computed(() => {
    return commonStore.idc;
  });

  const flow = computed(() => {
    return commonStore.flow;
  });

  const dbForm = ref({
    idc: '',
    source: '',
    ip: '',
    port: 3306,
    password: '',
    username: '',
    is_query: 2,
    flow_id: null as any,
    principal: '',
    ca_file: '',
    cert: '',
    key_file: '',
    db_type: 0,
    auth_database: '',
    mongo_replica_set: '',
    mongo_tls: false,
    mongo_auth_mechanism: '',
    rule_id: 0,
    environment_tag: '',
  } as Source);

  // 切换类型时带出该类型的默认端口（MySQL 3306 / PG 5432 / Mongo 27017 / Redis 6379 / Doris 9030）
  const onTypeChange = (v: number) => {
    dbForm.value.port = dbTypePort(v, dbForm.value.port);
    // 类型切换后旧规则集可能不再兼容，先回到全局规则，避免提交后才被后端拒绝。
    if (
      dbForm.value.rule_id !== 0 &&
      !isRuleSetCompatibleWithDBType(
        ruleSets.value.find((item) => item.id === dbForm.value.rule_id) || {
          dialect: '',
          audit_role: {},
        },
        v,
      )
    ) {
      dbForm.value.rule_id = 0;
    }
  };

  const formRef = ref();

  const resetFields = () => {
    formRef.value.resetFields();
  };

  const fill = (vl: Source) => {
    dbForm.value = vl;
  };

  const createSource = () => {
    formRef.value.validate().then(async () => {
      await cs({ db: dbForm.value, tp: 'create' });
      emit('created');
      resetFields();
    });
  };

  const checkConn = async () => {
    if (loading.value) return;
    loading.value = true;
    try {
      await cs({ db: dbForm.value, tp: 'test' });
    } finally {
      loading.value = false;
    }
  };

  defineExpose({
    fill,
  });
</script>

<style scoped>
  :deep(.ant-form-item) {
    margin-bottom: 18px;
  }
  :deep(.ant-input-number) {
    width: 100%;
  }
  .form-section-title {
    font-size: 13px;
    font-weight: 600;
    margin: 24px 0 16px;
    padding-top: 20px;
    border-top: 1px solid var(--frame-line);
    color: var(--text-muted);
  }
  .form-section-title:first-child {
    margin-top: 0;
    padding-top: 0;
    border-top: 0;
  }
</style>
