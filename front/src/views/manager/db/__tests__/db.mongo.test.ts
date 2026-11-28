import { describe, it, expect, vi } from 'vitest';
import { shallowMount, flushPromises } from '@vue/test-utils';
import Antd, { Form, FormItem, Modal, Switch, Checkbox } from 'ant-design-vue';
import i18n from '@/lang';
import { DBType, type Source } from '@/apis/db';
import DbForm from '../dbForm.vue';
import DbModal from '../dbModal.vue';

vi.mock('@/store', () => ({
  useCommonStore: () => ({ principal: [], idc: [], flow: [] }),
}));
vi.mock('@/apis/db', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/apis/db')>()),
  createSource: vi.fn().mockResolvedValue({ data: { code: 1200 } }),
}));
vi.mock('@/apis/rules', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/apis/rules')>()),
  getCustomRulesList: vi.fn().mockResolvedValue({ data: { payload: [] } }),
}));
vi.mock('@/apis/setting', () => ({
  getSettingInfo: vi.fn().mockResolvedValue({ data: { payload: {} } }),
}));
vi.mock('@/apis/flow', () => ({
  getFlowProfile: vi
    .fn()
    .mockResolvedValue({ data: { payload: { steps: [] } } }),
}));
vi.mock('@/apis/source', () => ({
  querySchemaList: vi.fn().mockResolvedValue({ data: { payload: [] } }),
}));

const globalOptions = { plugins: [Antd, i18n], renderStubDefaultSlot: true };

function source(overrides: Partial<Source> = {}): Source {
  return {
    db_type: DBType.MongoDB,
    ip: 'node-a:27017,node-b:27017',
    port: 27017,
    username: 'user',
    password: 'p@ss^word%',
    auth_database: 'admin',
    mongo_replica_set: 'rs-test',
    mongo_tls: false,
    mongo_auth_mechanism: '',
    exclude_db_list: '',
    insulate_word_list: '',
    ...overrides,
  } as Source;
}

describe('MongoDB datasource forms', () => {
  it('accepts special characters and requires a username when a password is supplied', async () => {
    const wrapper = shallowMount(DbForm, { global: globalOptions });
    (wrapper.vm as unknown as { fill: (value: Source) => void }).fill(source());
    await flushPromises();
    const rules = wrapper.findComponent(Form).props('rules') as any;
    expect(rules.username[0].required).toBe(true);
    await expect(
      rules.password[1].validator({}, 'p@ss^word%'),
    ).resolves.toBeUndefined();
    const names = wrapper
      .findAllComponents(FormItem)
      .map((item) => item.props('name'));
    expect(names).toContain('auth_database');
    expect(names).toContain('mongo_replica_set');
    wrapper.unmount();
  });

  it('allows an anonymous MongoDB configuration while keeping SQL credentials required', async () => {
    const wrapper = shallowMount(DbForm, { global: globalOptions });
    const form = wrapper.vm as unknown as { fill: (value: Source) => void };
    form.fill(source({ username: '', password: '' }));
    await flushPromises();
    let rules = wrapper.findComponent(Form).props('rules') as any;
    expect(rules.username[0].required).toBe(false);
    expect(rules.password[0].required).toBe(false);
    form.fill(source({ db_type: 0, username: '', password: '' }));
    await flushPromises();
    rules = wrapper.findComponent(Form).props('rules') as any;
    expect(rules.username[0].required).toBe(true);
    expect(rules.password[0].required).toBe(true);
    wrapper.unmount();
  });

  it('binds the MongoDB TLS switch to the submitted datasource', async () => {
    const wrapper = shallowMount(DbForm, { global: globalOptions });
    (wrapper.vm as unknown as { fill: (value: Source) => void }).fill(
      source({
        mongo_tls: true,
        mongo_auth_mechanism: 'SCRAM-SHA-1',
        ca_file: 'ca-pem',
      }),
    );
    await flushPromises();
    const tlsSwitch = wrapper
      .findAllComponents(Switch)
      .find((item) => item.props('checkedChildren') === 'SSL')!;
    expect(tlsSwitch.props('checked')).toBe(true);
    expect(wrapper.text()).toContain('PEM');
    const model = wrapper.findComponent(Form).props('model') as Source;
    expect(model.mongo_auth_mechanism).toBe('SCRAM-SHA-1');
    await tlsSwitch.vm.$emit('update:checked', false);
    await flushPromises();
    expect(model.mongo_tls).toBe(false);
    expect(model.ca_file).toBe('ca-pem');
    wrapper.unmount();
  });

  it('edits and submits the authentication database and replica set', async () => {
    const { createSource } = await import('@/apis/db');
    const wrapper = shallowMount(DbModal, {
      props: { rules: [] },
      global: globalOptions,
    });
    await (
      wrapper.vm as unknown as { fillInfo: (value: Source) => Promise<void> }
    ).fillInfo(
      source({
        mongo_tls: true,
        mongo_auth_mechanism: 'SCRAM-SHA-256',
        ca_file: 'ca-pem',
        cert: 'client-cert',
        key_file: 'client-key',
      }),
    );
    await flushPromises();
    const names = wrapper
      .findAllComponents(FormItem)
      .map((item) => item.props('name'));
    expect(names).toContain('auth_database');
    expect(names).toContain('mongo_replica_set');
    const model = wrapper.findComponent(Form).props('model') as Source;
    const tlsSwitch = wrapper
      .findAllComponents(Switch)
      .find((item) => item.props('checkedChildren') === 'SSL')!;
    expect(tlsSwitch.props('checked')).toBe(true);
    await tlsSwitch.vm.$emit('update:checked', false);
    model.mongo_auth_mechanism = '';
    model.auth_database = 'accounts';
    model.mongo_replica_set = '';
    await wrapper.findComponent(Modal).vm.$emit('ok');
    await flushPromises();
    expect(createSource).toHaveBeenCalledWith(
      expect.objectContaining({
        tp: 'edit',
        db: expect.objectContaining({
          auth_database: 'accounts',
          mongo_replica_set: '',
          mongo_tls: false,
          mongo_auth_mechanism: '',
          ca_file: 'ca-pem',
          cert: 'client-cert',
          key_file: 'client-key',
        }),
      }),
    );
    wrapper.unmount();
  });

  it('preserves list connection options and leaves the password unchanged by default', async () => {
    const { createSource } = await import('@/apis/db');
    const wrapper = shallowMount(DbModal, {
      props: { rules: [] },
      global: globalOptions,
    });
    const listed = source({
      password: '',
      auth_database: 'accounts',
      mongo_replica_set: 'rs-production',
      mongo_tls: true,
      mongo_auth_mechanism: 'SCRAM-SHA-256',
      ca_file: '',
      cert: '',
      key_file: '',
    });
    await (
      wrapper.vm as unknown as { fillInfo: (value: Source) => Promise<void> }
    ).fillInfo(listed);
    await flushPromises();
    const model = wrapper.findComponent(Form).props('model') as Source;
    model.source = 'renamed';
    await wrapper.findComponent(Modal).vm.$emit('ok');
    await flushPromises();
    expect(createSource).toHaveBeenLastCalledWith(
      expect.objectContaining({
        tp: 'edit',
        clear_password: false,
        db: expect.objectContaining({
          password: '',
          source: 'renamed',
          auth_database: 'accounts',
          mongo_replica_set: 'rs-production',
          mongo_tls: true,
          mongo_auth_mechanism: 'SCRAM-SHA-256',
        }),
      }),
    );
    wrapper.unmount();
  });

  it('clears a password only through the explicit control and resets it on reopening', async () => {
    const { createSource } = await import('@/apis/db');
    const wrapper = shallowMount(DbModal, {
      props: { rules: [] },
      global: globalOptions,
    });
    const modal = wrapper.vm as unknown as {
      fillInfo: (value: Source) => Promise<void>;
    };
    await modal.fillInfo(source({ password: '' }));
    await flushPromises();
    const model = wrapper.findComponent(Form).props('model') as Source;
    model.password = 'typed-but-cleared';
    model.username = '';
    await wrapper.findComponent(Checkbox).vm.$emit('update:checked', true);
    await wrapper.findComponent(Modal).vm.$emit('ok');
    await flushPromises();
    expect(createSource).toHaveBeenLastCalledWith(
      expect.objectContaining({
        tp: 'edit',
        clear_password: true,
        db: expect.objectContaining({ password: '', username: '' }),
      }),
    );
    await modal.fillInfo(source({ password: '' }));
    await flushPromises();
    expect(wrapper.findComponent(Checkbox).props('checked')).toBe(false);
    await wrapper.findComponent(Modal).vm.$emit('ok');
    await flushPromises();
    expect(createSource).toHaveBeenLastCalledWith(
      expect.objectContaining({ clear_password: false }),
    );
    wrapper.unmount();
  });
});

describe('Datasource connection feedback', () => {
  it.each([DbForm, DbModal])(
    'allows retry after a failed connection test',
    async (component) => {
      const { createSource } = await import('@/apis/db');
      vi.mocked(createSource).mockRejectedValueOnce(new Error('offline'));
      const wrapper = shallowMount(component, {
        props: { rules: [] },
        global: globalOptions,
      });
      await flushPromises();
      await expect((wrapper.vm as any).checkConn()).rejects.toThrow('offline');
      await flushPromises();
      await expect((wrapper.vm as any).checkConn()).resolves.toBeUndefined();
      wrapper.unmount();
    },
  );
});

describe('Datasource editing availability', () => {
  it('renders empty approval guidance when the flow has null steps', async () => {
    const { getFlowProfile } = await import('@/apis/flow');
    vi.mocked(getFlowProfile).mockResolvedValueOnce({
      data: { payload: { steps: null } },
    } as any);
    const wrapper = shallowMount(DbModal, {
      props: { rules: [] },
      global: globalOptions,
    });
    await (wrapper.vm as any).fillInfo(source());
    await flushPromises();
    expect(wrapper.text()).toContain(i18n.global.t('db.flow.empty'));
    wrapper.unmount();
  });

  it('keeps the editor available when the schema lookup fails', async () => {
    const { querySchemaList } = await import('@/apis/source');
    vi.mocked(querySchemaList).mockRejectedValueOnce(
      new Error('connection timed out'),
    );
    const wrapper = shallowMount(DbModal, {
      props: { rules: [] },
      global: globalOptions,
    });
    await expect(
      (wrapper.vm as any).fillInfo(source()),
    ).resolves.toBeUndefined();
    await flushPromises();
    expect(wrapper.findComponent(Modal).props('open')).toBe(true);
    expect((wrapper.findComponent(Form).props('model') as Source).ip).toBe(
      'node-a:27017,node-b:27017',
    );
    wrapper.unmount();
  });
});
