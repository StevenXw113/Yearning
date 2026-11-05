<template>
  <a-modal
    v-model:open="is_open"
    :title="$t('db.edit.title')"
    width="1000px"
    :ok-text="$t('common.save')"
    @ok="editDB"
  >
    <a-row>
      <a-col :span="15">
        <a-form :model="dbForm" v-bind="layout">
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
          <a-form-item :label="$t('db.addr')" name="ip">
            <a-input v-model:value="dbForm.ip"></a-input>
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
            ></a-input-password>
          </a-form-item>
          <!-- Redis：诊断命令（SLOWLOG/LATENCY/DUMP/COMMAND/INFO 全量）默认不放行，
               它们能看到实例配置或其它业务的执行信息 -->
          <a-form-item
            v-if="dbForm.db_type === DBType.Redis"
            :label="$t('db.allow.diag')"
          >
            <a-switch v-model:checked="dbForm.allow_diag_commands" />
            <div style="color: #8c8c8c; font-size: 12px; margin-top: 4px">
              {{ $t('db.allow.diag.tips') }}
            </div>
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
            <div style="color: #8c8c8c; font-size: 12px; margin-top: 4px">
              {{ $t('db.redis.mode.tips') }}
            </div>
          </a-form-item>
          <a-form-item :label="$t('db.kind')">
            <a-switch
              v-model:checked="checked"
              :checked-children="$t('db.ssl')"
              :un-checked-children="$t('user.password')"
            />
          </a-form-item>
          <template v-if="checked">
            <a-form-item :label="$t('db.ssl.ca')">
              <a-textarea v-model:value="dbForm.ca_file" :rows="5"></a-textarea>
            </a-form-item>
            <a-form-item :label="$t('db.ssl.certificate')">
              <a-textarea v-model:value="dbForm.cert" :rows="5"></a-textarea>
            </a-form-item>
            <a-form-item :label="$t('db.ssl.key')">
              <a-textarea
                v-model:value="dbForm.key_file"
                :rows="5"
              ></a-textarea>
            </a-form-item>
          </template>
          <a-form-item :label="$t('common.table.type')" name="is_query">
            <a-radio-group v-model:value="dbForm.is_query" name="radioGroup">
              <a-radio :value="2">{{ $t('db.rw') }}</a-radio>
              <a-radio :value="0">{{ $t('db.w') }}</a-radio>
              <a-radio :value="1">{{ $t('db.r') }}</a-radio>
            </a-radio-group>
          </a-form-item>
          <a-form-item :label="$t('menu.manage.flow')">
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
          <a-form-item :label="$t('common.rules')">
            <a-select v-model:value="dbForm.rule_id" style="width: 100%">
              <a-select-option :key="-1" :value="0">{{
                $t('db.global')
              }}</a-select-option>
              <a-select-option v-for="i in rules" :key="i.id" :value="i.id">{{
                i.desc
              }}</a-select-option>
            </a-select>
          </a-form-item>
          <a-form-item :label="$t('common.action')">
            <a-button type="primary" ghost :loading="loading" @click="checkConn"
              >{{ $t('db.test') }}
            </a-button>
          </a-form-item>
        </a-form>
      </a-col>
      <a-col :span="8" offset="1">
        <a-steps direction="vertical" size="small" :current="0">
          <a-step v-for="i in steps" :key="i.desc" :title="i.desc">
            <template #description>
              <a-tag v-for="k in i.auditor" :key="k" color="#108ee9">{{
                k
              }}</a-tag>
            </template>
          </a-step>
        </a-steps>
      </a-col>
    </a-row>
  </a-modal>
</template>

<script lang="ts" setup>
  import { getFlowProfile, Steps } from '@/apis/flow';
  import CommonMixins from '@/mixins/common';
  import { ref, computed } from 'vue';
  import { useCommonStore } from '@/store';
  import { createSource, dbTypeLabel, Source, DBType } from '@/apis/db';
  import { querySchemaList } from '@/apis/source';
  import { CustomRule } from '@/apis/rules';

  defineProps<{
    rules: CustomRule[];
  }>();

  // 编辑保存成功后通知父组件(刷新列表),替代原全局 EventBus('postOk')
  const emit = defineEmits<{ edited: [] }>();

  const { is_open, turnState, layout } = CommonMixins();

  const checked = ref(false);

  const commonStore = useCommonStore();

  const loading = ref(false);

  const idc = computed(() => {
    return commonStore.idc;
  });

  const flow = computed(() => {
    return commonStore.flow;
  });

  const steps = ref([] as Steps[]);

  const principalList = computed(() => commonStore.principal);

  let dbForm = ref({
    idc: '',
    source: '',
    ip: '',
    port: 0,
    password: '',
    username: '',
    is_query: 2,
    flow_id: 0,
    source_id: '',
    exclude_db_list: '',
    insulate_word_list: '',
    rule_id: 0,
    allow_diag_commands: false,
    redis_mode: '',
  } as Source);

  const schemaList = ref<string[]>([]);

  const excludeDB = ref<string[]>([]);

  const insulateWord = ref<string[]>([]);

  const mergeFlow = async (vl: number) => {
    const { data } = await getFlowProfile(vl);
    steps.value = data.payload.steps;
  };

  const editDB = async () => {
    dbForm.value.exclude_db_list = excludeDB.value.join(',');
    dbForm.value.insulate_word_list = insulateWord.value.join(',');
    await createSource({ db: dbForm.value, tp: 'edit' });
    turnState();
    emit('edited');
  };

  const fetchSchema = async () => {
    const { data } = await querySchemaList(dbForm.value.source_id);
    schemaList.value = data.payload;
  };

  const fillInfo = async (vl: any) => {
    dbForm.value = Object.assign({}, vl);
    turnState();
    excludeDB.value = dbForm.value.exclude_db_list.split(',');
    insulateWord.value = dbForm.value.insulate_word_list
      .split(',')
      .map((i: string) => i.trim())
      .filter(Boolean);
    fetchSchema();
    const { data } = await getFlowProfile(dbForm.value.flow_id);
    steps.value = data.payload.steps;
  };

  const checkConn = async () => {
    loading.value = !loading.value;
    await createSource({ db: dbForm.value, tp: 'test' });
    loading.value = !loading.value;
  };

  defineExpose({
    fillInfo,
  });
</script>
