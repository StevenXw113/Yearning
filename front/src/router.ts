import { createRouter, createWebHashHistory, RouteRecordRaw } from 'vue-router';
import NProgress from 'nprogress';
import { pinia, useUserStore } from './store';

// 惰性取实例：router 处于 store→apis→request→router 的循环依赖链上，
// 顶层立即实例化在部分加载顺序下会失败；守卫只在导航时执行。
const userStore = () => useUserStore(pinia);

NProgress.configure({ showSpinner: false });

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      meta: {
        title: 'Login - 登录',
      },
      component: () => import('@/views/login/login.vue'),
    },
    {
      path: '/',
      name: 'layout',
      meta: {
        title: 'Layout - layout',
      },
      component: () => import('@/views/layout/layout.vue'),
      redirect: '/home',
      children: [
        {
          path: '/home',
          name: 'home',
          meta: {
            title: 'Home - 工作台',
          },
          redirect: '/home/workplace',
          component: () => import('@/views/common/subLayout.vue'),
          children: [
            {
              path: '/home/workplace',
              name: 'workplace',
              meta: {
                title: 'Home - 工作台',
              },
              component: () => import('@/views/home/home.vue'),
            },
            {
              path: '/home/profile',
              name: 'profile',
              meta: {
                title: 'Profile - 个人详情',
              },
              component: () => import('@/views/home/profile.vue'),
            },
          ],
        },
        {
          path: '/advisor',
          name: 'advisor',
          meta: {
            title: 'AI 分析',
          },
          component: () => import('@/views/common/subLayout.vue'),
          redirect: '/advisor/ai',
          children: [
            {
              path: '/advisor/ai',
              name: 'advisor/ai',
              meta: {
                title: 'Advisor - AI 分析',
              },
              component: () => import('@/views/advisor/index.vue'),
            },
          ],
        },
        {
          path: '/apply',
          name: 'apply',
          meta: {
            title: 'Order - 工单',
          },
          redirect: '/apply/list',
          component: () => import('@/views/common/subLayout.vue'),
          children: [
            {
              path: '/apply/list',
              name: 'apply/list',
              meta: {
                title: 'Order - 工单申请列表',
              },
              component: () => import('@/views/apply/apply.vue'),
            },
            {
              path: '/apply/order',
              name: 'apply/order',
              meta: {
                title: 'Order - 工单填写',
              },
              component: () => import('@/views/apply/order.vue'),
            },
            {
              path: '/apply/batch',
              name: 'apply/batch',
              meta: {
                title: 'Order - 项目工单（批量）',
              },
              component: () => import('@/views/apply/batch.vue'),
            },
            {
              path: '/apply/query',
              name: 'apply/query',
              meta: {
                keepalive: true,
                title: 'Query - 查询',
              },
              component: () => import('@/views/query/query.vue'),
            },
          ],
        },
        {
          path: '/comptroller',
          name: 'comptroller',
          component: () => import('@/views/common/subLayout.vue'),
          children: [
            {
              path: '/comptroller/order/:tp',
              meta: {
                title: '审计',
              },
              component: () => import('@/views/record/record.vue'),
            },
          ],
        },
        {
          path: '/server',
          name: 'server',
          component: () => import('@/views/common/subLayout.vue'),
          children: [
            {
              path: '/server/order/audit',
              name: 'server/order/audit',
              meta: {
                title: '工单审核',
              },
              component: () => import('@/views/common/auditLayout.vue'),
            },
            {
              path: '/server/query/list',
              name: 'server/query/list',
              meta: {
                title: '查询审核',
                keepAlive: true,
              },
              component: () => import('@/views/server/query/list.vue'),
            },
            {
              path: '/server/order/:tp/list',
              meta: {
                title: '工单列表',
                keepAlive: true,
              },
              component: () => import('@/views/server/order/list.vue'),
            },
          ],
        },
        {
          path: '/manager',
          name: 'manager',
          component: () => import('@/views/common/subLayout.vue'),
          redirect: '/manager/user',
          children: [
            {
              path: '/manager/user',
              name: 'manager/user',
              meta: {
                title: 'User - 用户',
              },
              component: () => import('@/views/manager/user/user.vue'),
            },
            {
              path: '/manager/db',
              name: 'manager/db',
              meta: {
                title: 'DB - 数据源',
              },
              component: () => import('@/views/manager/db/db.vue'),
            },
            {
              path: '/manager/flow',
              name: 'manager/flow',
              meta: {
                title: 'Flow - 流程',
              },
              component: () => import('@/views/manager/flow/flow.vue'),
            },
            {
              path: '/manager/policy',
              name: 'manager/policy',
              meta: {
                title: 'Policy - 权限组',
              },
              component: () => import('@/views/manager/policy/policy.vue'),
            },
            {
              path: '/manager/rules',
              name: 'manager/rules',
              meta: {
                title: 'Rules - 审核规则',
              },
              component: () => import('@/views/manager/rules/index.vue'),
            },
            {
              path: '/manager/autotask',
              name: 'manager/autotask',
              meta: {
                title: 'AutoTask - 自动化任务',
              },
              component: () => import('@/views/manager/autotask/autotask.vue'),
            },
            {
              path: '/manager/board',
              name: 'manager/board',
              meta: {
                title: 'Board - 公告',
              },
              component: () => import('@/views/manager/board/board.vue'),
            },
            {
              path: '/manager/setting',
              name: 'manager/setting',
              meta: {
                title: 'Setting - 设置',
              },
              component: () => import('@/views/manager/setting/setting.vue'),
            },
          ],
        },
      ],
    },
  ] as Array<RouteRecordRaw>,
});

// vue-router 5:守卫改返回式(next 回调已弃用)。未登录直接重定向登录页,
// 不再需要 v4 的 next(false)+replace 两段式
router.beforeEach((to) => {
  NProgress.start();
  // user.account 可能为 null/undefined（storage 还原或 plugin 整体覆盖时的边界），用可选链保护
  const token = userStore().account?.token;
  if (!token && to.name !== 'login') {
    return { name: 'login' };
  }
});

router.afterEach(() => {
  NProgress.done();
});

export default router;
