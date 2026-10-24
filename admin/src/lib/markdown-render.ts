import DOMPurify from 'dompurify'
import MarkdownIt from 'markdown-it'
import abbr from 'markdown-it-abbr'
import container from 'markdown-it-container'
import deflist from 'markdown-it-deflist'
import { full as emoji } from 'markdown-it-emoji'
import footnote from 'markdown-it-footnote'
import ins from 'markdown-it-ins'
import mark from 'markdown-it-mark'
import sub from 'markdown-it-sub'
import sup from 'markdown-it-sup'
import taskLists from 'markdown-it-task-lists'

// The options and plugins of Strapi's preview, so that the same text reads
// the same in both admins; task lists come on top.
const md = new MarkdownIt({ html: true, xhtmlOut: false, breaks: true, langPrefix: 'language-', linkify: true, typographer: true })
  .use(abbr)
  .use(container, 'warning')
  .use(container, 'tip')
  .use(deflist)
  .use(emoji)
  .use(footnote)
  .use(ins)
  .use(mark)
  .use(sub)
  .use(sup)
  .use(taskLists)

// renderMarkdown is the preview's HTML. Raw HTML is allowed in the text,
// so the output is sanitized before it reaches the page.
export function renderMarkdown(text: string): string {
  return DOMPurify.sanitize(md.render(text))
}
