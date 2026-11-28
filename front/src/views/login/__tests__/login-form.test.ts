import { beforeEach, describe, expect, it, vi } from 'vitest';
import { shallowMount, flushPromises } from '@vue/test-utils';
import Antd, { Button, Form } from 'ant-design-vue';
import i18n from '@/lang';
import LoginForm from '../login-form.vue';

const mocks = vi.hoisted(() => ({
  signIn: vi.fn(),
  mfaSignIn: vi.fn(),
  store: vi.fn(),
  replace: vi.fn(),
}));
vi.mock('@/apis/loginApi', () => ({
  signIn: mocks.signIn,
  mfaSignIn: mocks.mfaSignIn,
  getOIDCState: vi
    .fn()
    .mockResolvedValue({ data: { code: 1200, payload: { enabled: false } } }),
}));
vi.mock('@/router', () => ({ default: { replace: mocks.replace } }));
vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }) }));
vi.mock('@vueuse/core', () => ({ useDebounceFn: (fn: unknown) => fn }));
vi.mock('@/store', () => ({
  useUserStore: () => ({ USER_STORE: mocks.store }),
  useMenuStore: () => ({ CHANGE_SELECTED: vi.fn() }),
}));
const mountForm = () =>
  shallowMount(LoginForm, {
    global: { plugins: [Antd, i18n], renderStubDefaultSlot: true },
  });
const submitButton = (wrapper: ReturnType<typeof mountForm>) =>
  wrapper
    .findAllComponents(Button)
    .find((button) => button.props('type') === 'primary')!;

beforeEach(() => vi.clearAllMocks());
describe('Login feedback', () => {
  it('prevents duplicate requests and releases loading after a failed request', async () => {
    let rejectRequest!: (reason: Error) => void;
    mocks.signIn.mockReturnValue(
      new Promise((_, reject) => {
        rejectRequest = reject;
      }),
    );
    const wrapper = mountForm();
    await flushPromises();
    const vm = wrapper.vm as any;
    const pending = vm.userSignIn();
    await flushPromises();
    expect(submitButton(wrapper).props('loading')).toBe(true);
    await vm.userSignIn();
    expect(mocks.signIn).toHaveBeenCalledTimes(1);
    const failure = expect(pending).rejects.toThrow('offline');
    rejectRequest(new Error('offline'));
    await failure;
    await flushPromises();
    expect(submitButton(wrapper).props('loading')).toBe(false);
    expect(mocks.store).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('keeps the MFA step and clears loading when verification fails', async () => {
    mocks.signIn.mockResolvedValue({
      data: {
        code: 1200,
        payload: {
          mfa_required: true,
          ticket: 'test-ticket',
          user: 'operator',
        },
      },
    });
    const wrapper = mountForm();
    await flushPromises();
    const vm = wrapper.vm as any;
    await vm.userSignIn();
    await flushPromises();
    const model = wrapper.findComponent(Form).props('model') as {
      code: string;
      ticket: string;
    };
    expect(model.ticket).toBe('test-ticket');
    model.code = '123456';
    mocks.mfaSignIn.mockRejectedValue(new Error('offline'));
    await expect(vm.mfaConfirm()).rejects.toThrow('offline');
    await flushPromises();
    expect(submitButton(wrapper).props('loading')).toBe(false);
    expect(wrapper.findComponent(Form).props('model')).toBe(model);
    expect(mocks.store).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('does not create a login session for an unsuccessful response', async () => {
    mocks.signIn.mockResolvedValue({ data: { code: 1301 } });
    const wrapper = mountForm();
    await flushPromises();
    await (wrapper.vm as any).userSignIn();
    expect(mocks.store).not.toHaveBeenCalled();
    expect(mocks.replace).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});
