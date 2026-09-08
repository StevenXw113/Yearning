import DOMPurify from 'dompurify';

// 渲染不可信富文本（服务端下发的公告/板内容、AI 输出等）前必须过滤，防存储型/注入型 XSS。
// 采用 DOMPurify 默认白名单（默认已剔除 script / on* 事件 / javascript: 等危险内容，
// 且不保留 iframe / object / embed / form），可最大程度保留 Vditor/markdown 排版。
export const sanitizeHTML = (html: string): string =>
  DOMPurify.sanitize(html, {
    FORBID_TAGS: ['iframe', 'object', 'embed', 'form'],
    FORBID_ATTR: ['style'],
    ALLOW_DATA_ATTR: false,
  });
