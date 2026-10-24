const acronyms = new Set(['api', 'css', 'faq', 'html', 'id', 'ip', 'json', 'seo', 'tldr', 'uid', 'url', 'uuid'])

// humanize turns a field name into words: hideOnHomepage reads "Hide on homepage".
export function humanize(name: string): string {
  const words = name
    .replace(/[_-]+/g, ' ')
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/([A-Z]+)([A-Z][a-z])/g, '$1 $2')
    .trim()
    .toLowerCase()
    .split(/\s+/)
    .map((w) => (acronyms.has(w) ? w.toUpperCase() : w))
    .join(' ')
  return words.charAt(0).toUpperCase() + words.slice(1)
}

// fieldLabel is the configured label, or the humanized name when the label
// was left to its default, the name itself.
export function fieldLabel(name: string, label?: string): string {
  return label && label !== name ? label : humanize(name)
}
