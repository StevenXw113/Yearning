import { isDark } from '@/style/theme';

// G2 v5 内置主题名（lib/core.js 里注册的是 light / dark / classic / classicDark / academy）。
// 不传 theme 时用 light：坐标轴、图例文字取 colorBlack=#1D2129，
// 落在暗色卡片（#1F262E）上几乎不可见，所以暗色必须显式换成 dark。
export const g2Theme = (): 'light' | 'dark' =>
  isDark.value === 'dark' ? 'dark' : 'light';

// 轴文字与卡片标题同色（对应 base.less 的 --chart-meta），Canvas 读不到 CSS 变量，只能在这里同步一份
export const g2LabelFill = (): string =>
  isDark.value === 'dark' ? 'rgba(255, 255, 255, 0.65)' : 'rgba(0, 0, 0, 0.65)';
