import { expect, test } from 'bun:test'
import { Window } from 'happy-dom'

test('renderMarkdown renders GFM and strips scripts', async () => {
  const w = new Window()
  Object.assign(globalThis, { window: w, document: w.document })
  const { renderMarkdown } = await import('./markdown-render')
  const html = renderMarkdown('# T\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n- [x] done\n\n`c`\n\n![i](/u.png)\n\nhttps://example.com')
  expect(html).toContain('<h1>T</h1>')
  expect(html).toContain('<table>')
  expect(html).toContain('type="checkbox"')
  expect(html).toContain('<code>c</code>')
  expect(html).toContain('<img src="/u.png" alt="i">')
  expect(html).toContain('<a href="https://example.com">')
  // Separate inputs: happy-dom swallows what follows a script element.
  expect(renderMarkdown('a\n\n<script>alert(1)</script>')).not.toContain('<script')
  expect(renderMarkdown('<img src=x onerror="alert(1)">')).not.toContain('onerror')
  expect(renderMarkdown('[x](javascript:alert(1))')).not.toContain('href="javascript')
})
