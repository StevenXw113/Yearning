import { createApp } from 'vue';
import App from './App.vue';
import routes from '@/router';
import Antd from 'ant-design-vue';
import { pinia } from '@/store/index';
import { setupSessionPersistence } from '@/store/persist';
import i18n from '@/lang';
import CTable from '@/components/table/index';
import { extend } from 'dayjs';
import weekday from 'dayjs/plugin/weekday';
import localeData from 'dayjs/plugin/localeData';

extend(weekday);
extend(localeData);

if (localStorage.getItem('theme') === null) {
  import('@/style/theme.less');
  localStorage.setItem('theme', 'dark');
} else {
  if (localStorage.getItem('theme') === 'dark') {
    import('@/style/theme.less');
  } else {
    import('@/style/light.less');
  }
}

createApp(App)
  .directive('watermark', (el, binding) => {
    addWaterMarker(binding.value.text);
  })
  .use(i18n)
  .use(pinia)
  .use(Antd)
  .use(CTable)
  .use(routes)
  .mount('#app');

// 诊断辅助：浏览器控制台可直接查看 store 状态
(window as any).__pinia = pinia;
// 会话持久化：user/menu/order 状态写回 sessionStorage 并在刷新时恢复
setupSessionPersistence();

function addWaterMarker(text: string) {
  const id = '1.23452384164.123412415';
  if (document.getElementById(id) !== null) {
    document.body.removeChild(document.getElementById(id) as HTMLElement);
  }
  const can = document.createElement('canvas');
  // 设置canvas画布大小
  can.width = 200;
  can.height = 100;

  const cans = can.getContext('2d') as CanvasRenderingContext2D;
  cans.rotate((-20 * Math.PI) / 180); // 水印旋转角度
  cans.font = '20px Vedana';
  cans.fillStyle = '#4A4A4A';
  cans.textAlign = 'center';
  cans.textBaseline = 'middle';
  cans.fillText(text, can.width / 2, can.height); // 水印在画布的位置x，y轴

  const div = document.createElement('div');
  div.id = id;
  div.style.pointerEvents = 'none';
  div.style.top = '40px';
  div.style.left = '0px';
  div.style.opacity = '0.1';
  div.style.position = 'fixed';
  div.style.zIndex = '100000';
  div.style.width = document.documentElement.clientWidth + 'px';
  div.style.height = document.documentElement.clientHeight + 'px';
  div.style.background =
    'url(' + can.toDataURL('image/png') + ') left top repeat';
  document.body.appendChild(div);
}
