import assert from 'node:assert/strict'
import { it } from 'node:test'
import { isDownloadReady } from '../src/data/site'
import { dictionaries, languages } from '../src/i18n'

function checkTranslation(reference: unknown, value: unknown, path: string) {
  if (typeof reference === 'string') {
    assert.equal(typeof value, 'string', path)
    assert.ok((value as string).trim(), `${path} must not be empty`)
    return
  }
  assert.ok(value && typeof value === 'object', path)
  if (Array.isArray(reference)) {
    assert.ok(Array.isArray(value), path)
    assert.equal(value.length, reference.length, `${path} must cover every control`)
    reference.forEach((item, index) => checkTranslation(item, value[index], `${path}.${index}`))
    return
  }
  for (const [key, item] of Object.entries(reference as Record<string, unknown>)) {
    checkTranslation(item, (value as Record<string, unknown>)[key], `${path}.${key}`)
  }
}

it('every published locale includes all content and accessible control labels', () => {
  assert.equal(new Set(languages.map(language => language.path)).size, 4)
  for (const language of languages) {
    checkTranslation(dictionaries.en, dictionaries[language.locale], language.locale)
  }
})

it('downloads stay disabled until a concrete HTTPS DMG URL is configured', () => {
  for (const url of [
    '',
    '#download',
    '/mumu.dmg',
    'javascript:alert(1)',
    'http://example.com/mumu.dmg',
    'https://example.com/releases',
    'https://example.com/mumu.zip',
  ]) {
    assert.equal(isDownloadReady(url), false, url)
  }
  assert.equal(isDownloadReady('https://example.com/releases/mumu-arm64.dmg'), true)
})
