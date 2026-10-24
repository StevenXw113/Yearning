// ESLint 9 flat config（自 .eslintrc.js 迁移）。
// 组合：vue3-recommended + typescript-eslint recommended + prettier。
// 与旧配置保持一致的关键裁决：no-explicit-any 关、no-unused-vars 警告、
// vue 模板排版类规则关（prettier 管格式）；import/* 插件未迁移——
// 其在 eslint 9 下无官方支持，原配置中的相关规则多为噪音。
import js from '@eslint/js';
import globals from 'globals';
import pluginVue from 'eslint-plugin-vue';
import tseslint from 'typescript-eslint';
import configPrettier from 'eslint-config-prettier';
import pluginPrettier from 'eslint-plugin-prettier';

export default tseslint.config(
  { ignores: ['dist/**', 'node_modules/**', '.*', '**/._*'] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...pluginVue.configs['flat/recommended'],
  configPrettier,
  {
    files: ['**/*.vue'],
    languageOptions: {
      parserOptions: { parser: tseslint.parser },
      // <script> 段无类型检查,js.recommended 的 no-undef 需要 browser 全局表
      // (TS 文件被 tseslint 的 eslint-recommended 覆盖关掉 no-undef,不受影响)
      globals: { ...globals.browser },
    },
  },
  {
    plugins: { prettier: pluginPrettier },
    languageOptions: {
      globals: { process: 'readonly' }, // 配置文件自身用 process.env 判定 debugger 级别
    },
    rules: {
      'prettier/prettier': 'warn',
      // 与原 .eslintrc.js 的既有裁决一致
      'vue/require-default-prop': 'off',
      'vue/singleline-html-element-content-newline': 'off',
      'vue/max-attributes-per-line': 'off',
      'vue/multi-word-component-names': 'off',
      'vue/no-v-html': 'off',
      'vue/html-indent': 'off', // 缩进交给 prettier（vueIndentScriptAndStyle）
      'vue/html-self-closing': 'off',
      'vue/attributes-order': 'off',
      'vue/first-attribute-linebreak': 'off',
      'vue/html-closing-bracket-newline': 'off',
      'vue/attribute-hyphenation': 'off', // 存量大量 v-model:activeKey 写法，改名属噪音
      '@typescript-eslint/ban-ts-comment': 'off',
      '@typescript-eslint/no-unused-vars': 'warn',
      '@typescript-eslint/no-empty-function': 'warn',
      '@typescript-eslint/no-explicit-any': 'off',
      // 存量代码广泛使用三元表达式作语句（sel !== '' ? a() : b()），是本仓库风格
      '@typescript-eslint/no-unused-expressions': 'off',
      // 存量类型断言 as {} 与空对象默认值；纯类型层遗留，无运行时风险
      '@typescript-eslint/no-empty-object-type': 'off',
      'no-debugger': process.env.NODE_ENV === 'production' ? 'error' : 'off',
      'no-param-reassign': 'off',
    },
  }
);
