import { expect, test } from 'bun:test'
import { JSDOM } from 'jsdom'

test('renderMarkdown renders GFM and strips scripts', async () => {
  // jsdom, not happy-dom: DOMPurify 3.4 does not sanitize under happy-dom 20+.
  const w = new JSDOM('').window
  Object.assign(globalThis, { window: w, document: w.document })
  const { renderMarkdown } = await import('./markdown-render')
  const html = renderMarkdown('# T\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n- [x] done\n\n`c`\n\n![i](/u.png)\n\nhttps://example.com')
  expect(html).toContain('<h1>T</h1>')
  expect(html).toContain('<table>')
  expect(html).toContain('type="checkbox"')
  expect(html).toContain('<code>c</code>')
  expect(html).toContain('<img src="/u.png" alt="i">')
  expect(html).toContain('<a href="https://example.com">')
  expect(renderMarkdown('a\n\n<script>alert(1)</script>')).not.toContain('<script')
  expect(renderMarkdown('<img src=x onerror="alert(1)">')).not.toContain('onerror')
  expect(renderMarkdown('[x](javascript:alert(1))')).not.toContain('href="javascript')
})
