import DOMPurify from 'dompurify';

// 渲染不可信富文本（如服务端下发的公告/板内容、AI 输出）前必须过一遍白名单过滤，
// 防止存储型/注入型 XSS。Vditor 生成的 markdown HTML 主要用到这些标签，故无需放开 script 等。
const BOARD_HTML_ALLOW: string[] = [
  'p',
  'br',
  'hr',
  'strong',
  'b',
  'em',
  'i',
  'del',
  's',
  'u',
  'sub',
  'sup',
  'span',
  'blockquote',
  'pre',
  'code',
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'ul',
  'ol',
  'li',
  'a',
  'img',
  'table',
  'thead',
  'tbody',
  'tr',
  'th',
  'td',
  'input',
];

export const sanitizeHTML = (html: string): string =>
  DOMPurify.sanitize(html, {
    ALLOWED_TAGS: BOARD_HTML_ALLOW,
    ALLOWED_ATTR: ['href', 'target', 'rel', 'src', 'alt', 'title', 'checked', 'type'],
    ALLOW_DATA_ATTR: false,
    // 仅允许 http(s):/ 开头的相对与绝对链接，杜绝 javascript:/data: 协议注入
    ALLOWED_URI_REGEXP: /^(?:(?:https?|mailto|ftp):|(?:\/|#))/i,
  });
