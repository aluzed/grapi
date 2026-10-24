import { expect, test } from 'bun:test'
import { fieldLabel, humanize } from './humanize'

test('humanize', () => {
  expect(humanize('hideOnHomepage')).toBe('Hide on homepage')
  expect(humanize('generateTldr')).toBe('Generate TLDR')
  expect(humanize('affiliated_with')).toBe('Affiliated with')
  expect(humanize('url')).toBe('URL')
  expect(humanize('HTMLContent')).toBe('HTML content')
  expect(humanize('title')).toBe('Title')
})

test('fieldLabel keeps a configured label', () => {
  expect(fieldLabel('hideOnHomepage', 'hideOnHomepage')).toBe('Hide on homepage')
  expect(fieldLabel('hideOnHomepage', 'Masquer')).toBe('Masquer')
  expect(fieldLabel('slug')).toBe('Slug')
})
