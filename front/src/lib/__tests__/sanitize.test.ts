import { describe, it, expect } from 'vitest';
import { sanitizeHTML } from '@/lib/sanitize';

// XSS 过滤是安全底线：公告/AI 输出等不可信富文本渲染前都走这里。
// 锁死四类攻击向量 + 基本排版保留。
describe('sanitizeHTML', () => {
  it('剔除 script 标签（内容降级为纯文本，不再可执行）', () => {
    const out = sanitizeHTML('<script>alert(1)</script>ok');
    expect(out).not.toContain('<script');
    expect(out).toContain('ok');
  });

  it('剔除内联事件属性(onerror/onload)', () => {
    const out = sanitizeHTML('<img src="x" onerror="alert(1)">');
    expect(out).not.toContain('onerror');
  });

  it('剔除 javascript: 协议', () => {
    const out = sanitizeHTML('<a href="javascript:alert(1)">x</a>');
    expect(out).not.toContain('javascript:');
  });

  it('剔除 FORBID_TAGS 清单(iframe/object/embed/form)', () => {
    expect(sanitizeHTML('<iframe src="https://evil"></iframe>')).not.toContain(
      'iframe',
    );
    expect(
      sanitizeHTML('<form action="https://evil"><input></form>'),
    ).not.toContain('<form');
  });

  it('剔除 style 与 data-* 属性（文本内容保留）', () => {
    const out = sanitizeHTML('<p style="color:red" data-x="1">t</p>');
    expect(out).not.toContain('style');
    expect(out).not.toContain('data-x');
    expect(out).toContain('t');
  });

  it('保留正常排版标签', () => {
    expect(sanitizeHTML('<p>hello <b>world</b></p>')).toContain('<b>world</b>');
  });
});
