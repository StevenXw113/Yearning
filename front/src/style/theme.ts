import { ref } from 'vue';
import { theme } from 'ant-design-vue';
import type { ThemeConfig as AntdThemeConfig } from 'ant-design-vue/es/config-provider/context';

// antd-vue 4 的主题体系：CSS-in-JS token + algorithm 运行时切换。
// 原 v3 是两份完整 less（antd.dark.less/antd.less + 变量覆盖）构建期切换。
// 业务 CSS（框架配色等）在 base.less 里以 :root[data-theme] CSS 变量承载，
// antd 组件的差异在这里用 token 承载；两者由同一个 isDark 驱动。

export type ThemeName = 'dark' | 'light';

const THEME_KEY = 'theme';

export const isDark = ref<ThemeName>(
  (localStorage.getItem(THEME_KEY) as ThemeName) || 'dark',
);

export function applyTheme(name: ThemeName) {
  isDark.value = name;
  localStorage.setItem(THEME_KEY, name);
  document.documentElement.dataset.theme = name;
  themeConfig.value = buildThemeConfig(name);
}

export function toggleTheme() {
  applyTheme(isDark.value === 'dark' ? 'light' : 'dark');
}

// 原 theme.less / light.less 的 antd 变量覆盖 → token 映射。
// 两主题共有的差异写进 token；仅暗色特有的写进 algorithm 分支。
// components 级 token 的类型声明在 antd-vue 4 中过宽（键名被并入 AliasToken 检查），
// 运行时按组件名 PascalCase 生效；这里收窄为 ThemeConfig 后断言。
type ThemeConfig = Omit<AntdThemeConfig, 'components'> & {
  components?: Record<string, Record<string, string | number>>;
};

function buildThemeConfig(name: ThemeName): ThemeConfig {
  if (name === 'dark') {
    return {
      algorithm: theme.darkAlgorithm,
      token: {
        colorPrimary: '#527590',
        colorError: '#EB175D',
        colorBgLayout: '#1F262E',
        colorBgContainer: '#1F262E',
        colorBgElevated: 'rgb(32, 37, 46)',
        colorBorderSecondary: '#3E4451',
        borderRadius: 4,
      },
      components: {
        Table: { headerBg: '#2A323D' },
        Card: { headBg: '#2A323D', bodyPadding: 16 },
        Menu: { itemSelectedBg: '#527590' },
        Tabs: {
          itemSelectedColor: '#FF9900',
          inkBarColor: '#FF9900',
          itemHoverColor: '#FF9900',
        },
        Transfer: { listHeaderBg: 'rgb(32, 37, 46)' },
      },
    };
  }
  return {
    algorithm: theme.defaultAlgorithm,
    token: {
      colorPrimary: 'rgb(38, 111, 232)',
      colorError: '#e34d59',
      colorBgLayout: '#F2F3F5',
      colorBgContainer: '#fff',
      borderRadius: 4,
    },
    components: {
      Menu: { itemSelectedBg: 'rgb(38, 111, 232)' },
    },
  };
}

export const themeConfig = ref<ThemeConfig>(buildThemeConfig(isDark.value));
