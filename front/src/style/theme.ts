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
  const dark = name === 'dark';
  return {
    algorithm: dark ? theme.darkAlgorithm : theme.defaultAlgorithm,
    token: {
      colorPrimary: dark ? '#6B9EFF' : '#245DD8',
      colorInfo: dark ? '#6B9EFF' : '#245DD8',
      colorError: dark ? '#FF7875' : '#C93645',
      colorBgLayout: dark ? '#101722' : '#F3F5F9',
      colorBgContainer: dark ? '#192332' : '#FFFFFF',
      colorBgElevated: dark ? '#202D3E' : '#FFFFFF',
      colorText: dark ? '#E6EDF7' : '#1D2939',
      colorTextSecondary: dark ? '#A5B4C8' : '#526174',
      colorBorderSecondary: dark ? '#304056' : '#E1E7F0',
      borderRadius: 8,
      fontFamily:
        '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
      fontSize: 14,
      controlHeight: 36,
    },
    components: {
      Table: { headerBg: dark ? '#202D3E' : '#F7F9FC', cellPaddingBlock: 13 },
      Card: { bodyPadding: 20, headerFontSize: 15 },
      Menu: {
        itemSelectedBg: dark ? '#253D62' : '#EAF0FF',
        itemSelectedColor: dark ? '#BDD4FF' : '#245DD8',
        itemHeight: 42,
      },
    },
  };
}

export const themeConfig = ref<ThemeConfig>(buildThemeConfig(isDark.value));
